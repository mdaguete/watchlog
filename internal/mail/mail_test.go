package mail

import "testing"

func TestParseURL_Full(t *testing.T) {
	cfg, err := ParseURL("smtps://user:pass123@smtp.example.com:465/noreply@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "smtp.example.com" { t.Errorf("host = %q", cfg.Host) }
	if cfg.Port != "465" { t.Errorf("port = %q", cfg.Port) }
	if cfg.User != "user" { t.Errorf("user = %q", cfg.User) }
	if cfg.Password != "pass123" { t.Errorf("password = %q", cfg.Password) }
	if cfg.From != "noreply@example.com" { t.Errorf("from = %q", cfg.From) }
	if !cfg.TLS { t.Error("expected TLS=true") }
}

func TestParseURL_SMTP(t *testing.T) {
	cfg, err := ParseURL("smtp://myuser:secret@mail.host.com:587/sender@host.com")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "mail.host.com" { t.Errorf("host = %q", cfg.Host) }
	if cfg.Port != "587" { t.Errorf("port = %q", cfg.Port) }
	if cfg.User != "myuser" { t.Errorf("user = %q", cfg.User) }
	if cfg.Password != "secret" { t.Errorf("password = %q", cfg.Password) }
	if cfg.From != "sender@host.com" { t.Errorf("from = %q", cfg.From) }
	if cfg.TLS { t.Error("expected TLS=false") }
}

func TestParseURL_DefaultPorts(t *testing.T) {
	cfg, _ := ParseURL("smtp://host.com/from@x.com")
	if cfg.Port != "587" { t.Errorf("smtp default port = %q, want 587", cfg.Port) }

	cfg, _ = ParseURL("smtps://host.com/from@x.com")
	if cfg.Port != "465" { t.Errorf("smtps default port = %q, want 465", cfg.Port) }
}

func TestParseURL_NoAuth(t *testing.T) {
	cfg, err := ParseURL("smtp://relay.internal:25/app@internal.local")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.User != "" { t.Errorf("user = %q, want empty", cfg.User) }
	if cfg.Password != "" { t.Errorf("password = %q, want empty", cfg.Password) }
	if cfg.Host != "relay.internal" { t.Errorf("host = %q", cfg.Host) }
	if cfg.Port != "25" { t.Errorf("port = %q", cfg.Port) }
}

func TestParseURL_InvalidScheme(t *testing.T) {
	_, err := ParseURL("http://host.com/from@x.com")
	if err == nil {
		t.Fatal("expected error for http scheme")
	}
}

func TestParseURL_Invalid(t *testing.T) {
	_, err := ParseURL("://bad")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFormatURL(t *testing.T) {
	cfg := Config{Host: "smtp.example.com", Port: "465", User: "user", Password: "secret", From: "noreply@example.com", TLS: true}
	url := FormatURL(cfg)
	if url != "smtps://user:••••••••@smtp.example.com:465/noreply@example.com" {
		t.Errorf("FormatURL = %q", url)
	}
}

func TestFormatURL_NoAuth(t *testing.T) {
	cfg := Config{Host: "relay.local", Port: "25", From: "app@local", TLS: false}
	url := FormatURL(cfg)
	if url != "smtp://relay.local:25/app@local" {
		t.Errorf("FormatURL = %q", url)
	}
}

func TestConfigured(t *testing.T) {
	cfg := Config{}
	if cfg.Configured() { t.Error("empty config should not be configured") }

	cfg = Config{Host: "h", Port: "25", From: "f@x"}
	if !cfg.Configured() { t.Error("should be configured") }
}
