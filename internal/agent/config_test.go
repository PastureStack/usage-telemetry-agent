package agent

import (
	"bytes"
	"strings"
	"testing"
)

func lookup(values map[string]string) envLookup {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func TestParseConfigRequiresExplicitPublishingTarget(t *testing.T) {
	values := map[string]string{
		"CATTLE_URL":         "http://127.0.0.1:8081/v2-beta",
		"CATTLE_ACCESS_KEY":  "access",
		"CATTLE_SECRET_KEY":  "secret",
		"TELEMETRY_TO_URL":   "https://retired.example.invalid/publish",
		"TELEMETRY_LISTEN":   "127.0.0.1:8114",
		"TELEMETRY_INTERVAL": "6h",
	}
	cfg, err := parseConfig(nil, lookup(values), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TargetURL != "" {
		t.Fatalf("legacy target unexpectedly enabled publishing: %q", cfg.TargetURL)
	}
}

func TestParseConfigAcceptsNewNamesAndLoopback(t *testing.T) {
	values := map[string]string{
		"PASTURESTACK_API_URL":                                 "http://127.0.0.1:8081/v2-beta",
		"PASTURESTACK_API_ACCESS_KEY":                          "access",
		"PASTURESTACK_API_SECRET_KEY":                          "secret",
		"PASTURESTACK_USAGE_TELEMETRY_TARGET_URL":              "https://metrics.example.invalid/ingest",
		"PASTURESTACK_USAGE_TELEMETRY_INCLUDE_INSTALLATION_ID": "true",
	}
	cfg, err := parseConfig([]string{"--interval", "1m"}, lookup(values), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IncludeInstallationID || cfg.Listen != defaultListen {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestParseConfigRejectsRemoteListenAndInsecureTarget(t *testing.T) {
	values := map[string]string{
		"PASTURESTACK_API_URL":        "http://127.0.0.1:8081/v2-beta",
		"PASTURESTACK_API_ACCESS_KEY": "access",
		"PASTURESTACK_API_SECRET_KEY": "secret",
	}
	for _, args := range [][]string{
		{"--listen", "0.0.0.0:8114"},
		{"--target-url", "http://metrics.example.invalid/ingest"},
		{"--url", "http://control-plane.example.invalid/v2-beta"},
		{"--url", "https://control-plane.example.invalid/v2-beta?token=unsafe"},
	} {
		_, err := parseConfig(args, lookup(values), &bytes.Buffer{})
		if err == nil {
			t.Fatalf("expected rejection for %v", args)
		}
	}
}

func TestValidateTargetRejectsCredentials(t *testing.T) {
	err := validateTargetURL("https://user:secret@metrics.example.invalid/ingest")
	if err == nil || !strings.Contains(err.Error(), "user information") {
		t.Fatalf("unexpected error: %v", err)
	}
}
