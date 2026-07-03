package mail

import (
	"fmt"
	"net/smtp"
	"strings"
)

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
}

func (c Config) Configured() bool {
	return c.Host != "" && c.Port != "" && c.From != ""
}

func Send(cfg Config, to, subject, body string) error {
	if !cfg.Configured() {
		return fmt.Errorf("SMTP not configured")
	}

	addr := cfg.Host + ":" + cfg.Port

	headers := []string{
		"From: " + cfg.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=\"UTF-8\"",
	}
	msg := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body)

	var auth smtp.Auth
	if cfg.User != "" {
		auth = smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)
	}

	return smtp.SendMail(addr, auth, cfg.From, []string{to}, msg)
}
