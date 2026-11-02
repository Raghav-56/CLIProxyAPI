package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodexAccountPolicyConfigRoundTrip(t *testing.T) {
	if err := ValidateV8Config([]byte("config-version: 8\nrouting: {codex-oauth-auth-id: subscription-b, codex-oauth-strategy: soonest-reset}\n")); err != nil {
		t.Fatalf("v8 schema rejected policy: %v", err)
	}
	for _, v8 := range []bool{false, true} {
		raw := []byte("routing: {strategy: fill-first, codex-oauth-auth-id: subscription-b, codex-oauth-strategy: soonest-reset}\n")
		cfg, err := ParseConfigBytes(raw)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Routing.CodexOAuthAuthID != "subscription-b" {
			t.Fatal("policy not parsed")
		}
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err = SaveConfigPreserveComments(path, cfg, v8); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadConfig(path)
		if err != nil || loaded.Routing.CodexOAuthAuthID != "subscription-b" || loaded.Routing.Strategy != "fill-first" || loaded.Routing.CodexOAuthStrategy != "soonest-reset" {
			t.Fatalf("policy round trip v8=%v: %v %v", v8, loaded, err)
		}
	}
}

func TestCodexAccountStrategyRejectsInvalidConfiguration(t *testing.T) {
	for _, strategy := range []string{"unknown", "round-robin", "SOONEST-RESET", "[]"} {
		raw := []byte("config-version: 8\nrouting: {codex-oauth-strategy: " + strategy + "}\n")
		if err := ValidateV8Config(raw); err == nil {
			t.Fatalf("v8 accepted invalid strategy %s", strategy)
		}
		if _, err := ParseConfigBytes(raw); err == nil {
			t.Fatalf("parser accepted invalid strategy %s", strategy)
		}
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(path); err == nil {
			t.Fatalf("loader accepted invalid strategy %s", strategy)
		}
	}
}
