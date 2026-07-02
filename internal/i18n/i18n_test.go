package i18n

import "testing"

func TestT_ExistingKey(t *testing.T) {
	got := T("es", "nav.shows")
	if got != "Series" {
		t.Errorf("T(es, nav.shows) = %q, want %q", got, "Series")
	}
	got = T("en", "nav.shows")
	if got != "Shows" {
		t.Errorf("T(en, nav.shows) = %q, want %q", got, "Shows")
	}
}

func TestT_MissingKey(t *testing.T) {
	got := T("es", "nonexistent.key")
	if got != "nonexistent.key" {
		t.Errorf("T(es, nonexistent.key) = %q, want key echoed back", got)
	}
}

func TestT_MissingLangFallsBackToES(t *testing.T) {
	got := T("fr", "nav.shows")
	if got != "Series" {
		t.Errorf("T(fr, nav.shows) = %q, want Spanish fallback %q", got, "Series")
	}
}

func TestT_AllKeysHaveBothLanguages(t *testing.T) {
	for key, langs := range translations {
		if _, ok := langs["es"]; !ok {
			t.Errorf("key %q missing Spanish translation", key)
		}
		if _, ok := langs["en"]; !ok {
			t.Errorf("key %q missing English translation", key)
		}
	}
}

func TestDetectLang_English(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"en-US,en;q=0.9", "en"},
		{"en", "en"},
		{"en-GB,es;q=0.5", "en"},
	}
	for _, tt := range tests {
		got := DetectLang(tt.header)
		if got != tt.want {
			t.Errorf("DetectLang(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestDetectLang_Spanish(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"es-ES,es;q=0.9,en;q=0.5", "es"},
		{"es", "es"},
		{"", "es"},
		{"fr-FR", "es"},
	}
	for _, tt := range tests {
		got := DetectLang(tt.header)
		if got != tt.want {
			t.Errorf("DetectLang(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}
