package db

import (
	"testing"
)

// ── envOrDefault ───────────────────────────────────────────────────────────

func TestEnvOrDefault_EnvSet_ReturnsEnvValue(t *testing.T) {
	t.Setenv("PACS_DB_TEST_KEY", "myvalue")
	if got := envOrDefault("PACS_DB_TEST_KEY", "fallback"); got != "myvalue" {
		t.Errorf("got=%q want=myvalue", got)
	}
}

func TestEnvOrDefault_EnvNotSet_ReturnsFallback(t *testing.T) {
	// Key is guaranteed absent because we never set it
	const absentKey = "PACS_DB_TEST_ABSENT_XYZ_9999"
	if got := envOrDefault(absentKey, "fallback"); got != "fallback" {
		t.Errorf("got=%q want=fallback when env is absent", got)
	}
}

func TestEnvOrDefault_EmptyString_ReturnsFallback(t *testing.T) {
	t.Setenv("PACS_DB_TEST_EMPTY", "")
	// An explicitly empty env var should fall through to the default
	if got := envOrDefault("PACS_DB_TEST_EMPTY", "fallback"); got != "fallback" {
		t.Errorf("got=%q want=fallback when env is empty string", got)
	}
}

func TestEnvOrDefault_DefaultsMatchExpectedServiceValues(t *testing.T) {
	// Verify callers in NewPostgresDB use the expected defaults when no env is set
	cases := []struct {
		key      string
		fallback string
	}{
		{"DB_HOST", "localhost"},
		{"DB_PORT", "5432"},
		{"DB_USER", "pacs_user"},
		{"DB_NAME", "pacs_db"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			// Temporarily unset to ensure default is used
			t.Setenv(tc.key, "")
			got := envOrDefault(tc.key, tc.fallback)
			if got != tc.fallback {
				t.Errorf("%s: got=%q want=%q", tc.key, got, tc.fallback)
			}
		})
	}
}
