package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"net/url"
	"strings"
)

// Config holds SMTP connection settings.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
	TLS      bool // true for smtps:// (implicit TLS on port 465)
}

// ParseURL parses a SMTP URL into a Config.
// Formats:
//
//	smtp://user:password@host:port/from@example.com
//	smtps://user:password@host:port/from@example.com
//	smtp://host:port/from@example.com  (no auth)
func ParseURL(raw string) (Config, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SMTP URL: %w", err)
	}

	var cfg Config

	switch u.Scheme {
	case "smtp":
		cfg.TLS = false
	case "smtps":
		cfg.TLS = true
	default:
		return Config{}, fmt.Errorf("unsupported scheme %q (use smtp:// or smtps://)", u.Scheme)
	}

	cfg.Host = u.Hostname()
	cfg.Port = u.Port()
	if cfg.Port == "" {
		if cfg.TLS {
			cfg.Port = "465"
		} else {
			cfg.Port = "587"
		}
	}

	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}

	// From address is in the path
	from := strings.TrimPrefix(u.Path, "/")
	if from != "" {
		cfg.From = from
	}

	return cfg, nil
}

// FormatURL converts a Config back to a URL string (password masked).
func FormatURL(cfg Config) string {
	scheme := "smtp"
	if cfg.TLS {
		scheme = "smtps"
	}
	var userInfo string
	if cfg.User != "" {
		userInfo = cfg.User + ":••••••••@"
	}
	from := ""
	if cfg.From != "" {
		from = "/" + cfg.From
	}
	return fmt.Sprintf("%s://%s%s:%s%s", scheme, userInfo, cfg.Host, cfg.Port, from)
}

// Configured returns true if enough settings are present to send email.
func (c Config) Configured() bool {
	return c.Host != "" && c.Port != "" && c.From != ""
}

// Send sends an HTML email.
func Send(cfg Config, to, subject, body string) error {
	if !cfg.Configured() {
		return fmt.Errorf("SMTP not configured")
	}

	headers := []string{
		"From: " + cfg.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=\"UTF-8\"",
	}
	msg := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body)

	addr := net.JoinHostPort(cfg.Host, cfg.Port)

	if cfg.TLS {
		return sendTLS(cfg, addr, to, msg)
	}
	return sendSTARTTLS(cfg, addr, to, msg)
}

// sendTLS connects with implicit TLS (port 465).
func sendTLS(cfg Config, addr, to string, msg []byte) error {
	tlsConfig := &tls.Config{ServerName: cfg.Host}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("SMTP client: %w", err)
	}
	defer client.Close()

	if cfg.User != "" {
		auth := smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}

	if err := client.Mail(cfg.From); err != nil {
		return fmt.Errorf("SMTP MAIL: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	w.Write(msg)
	w.Close()
	return client.Quit()
}

// sendSTARTTLS uses STARTTLS on port 587 (or plain if unavailable).
func sendSTARTTLS(cfg Config, addr, to string, msg []byte) error {
	var auth smtp.Auth
	if cfg.User != "" {
		auth = smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)
	}
	return smtp.SendMail(addr, auth, cfg.From, []string{to}, msg)
}
