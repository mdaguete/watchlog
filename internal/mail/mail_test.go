package mail

import "testing"

func TestConfig_Configured(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"empty", Config{}, false},
		{"partial", Config{Host: "smtp.example.com"}, false},
		{"no from", Config{Host: "smtp.example.com", Port: "587"}, false},
		{"full", Config{Host: "smtp.example.com", Port: "587", From: "a@b.com"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.Configured(); got != tt.want {
				t.Errorf("Configured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSend_NotConfigured(t *testing.T) {
	err := Send(Config{}, "to@example.com", "subject", "body")
	if err == nil {
		t.Error("expected error for unconfigured SMTP")
	}
}
