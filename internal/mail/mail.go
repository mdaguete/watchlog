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

// wrapEmailHTML wraps email body content in a styled HTML template.
func wrapEmailHTML(subject, body string) string {
	return `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>` + subject + `</title>
</head>
<body style="margin:0;padding:0;background-color:#ffffff;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#ffffff;">
<tr><td align="center" style="padding:40px 20px;">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
<!-- Header -->
<tr><td style="padding-bottom:32px;border-bottom:1px solid #e5e5e5;">
<span style="font-size:14px;font-weight:600;letter-spacing:2px;text-transform:uppercase;color:#000000;">WatchLog</span>
</td></tr>
<!-- Content -->
<tr><td style="padding:32px 0;color:#1a1a1a;font-size:15px;line-height:1.6;">
` + body + `
</td></tr>
<!-- Footer -->
<tr><td style="padding-top:32px;border-top:1px solid #e5e5e5;">
<p style="margin:0;font-size:12px;color:#999999;">WatchLog</p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`
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
	htmlBody := wrapEmailHTML(subject, body)
	msg := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + htmlBody)

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
