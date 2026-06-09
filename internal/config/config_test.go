package config

import (
	"testing"
	"time"
)

func TestLoadMarksDefaultSecretFromEnvironment(t *testing.T) {
	t.Setenv("GOV2_CONFIG", t.TempDir()+"/missing.json")
	t.Setenv("GOV2_TOKEN_SECRET", defaults().Security.TokenSecret)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Security.UsingDefaultSecret {
		t.Fatal("expected default token secret from environment to be marked unsafe")
	}
}

func TestLoadMarksCustomSecretFromEnvironment(t *testing.T) {
	t.Setenv("GOV2_CONFIG", t.TempDir()+"/missing.json")
	t.Setenv("GOV2_TOKEN_SECRET", "custom-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Security.UsingDefaultSecret {
		t.Fatal("expected custom token secret from environment to be marked safe")
	}
}

func TestLoadAppliesEnvironmentOverride(t *testing.T) {
	t.Setenv("GOV2_CONFIG", t.TempDir()+"/missing.json")
	t.Setenv("GOV2_ENVIRONMENT", "production")
	t.Setenv("GOV2_APP_NAME", "GOV2 Ops")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.App.Environment != "production" {
		t.Fatalf("expected production environment, got %q", cfg.App.Environment)
	}
	if cfg.App.Name != "GOV2 Ops" {
		t.Fatalf("expected app name override, got %q", cfg.App.Name)
	}
}

func TestLoadAppliesDurationAndIssuerOverrides(t *testing.T) {
	t.Setenv("GOV2_CONFIG", t.TempDir()+"/missing.json")
	t.Setenv("GOV2_SERVER_READ_TIMEOUT", "7s")
	t.Setenv("GOV2_SERVER_WRITE_TIMEOUT", "11s")
	t.Setenv("GOV2_SERVER_IDLE_TIMEOUT", "90s")
	t.Setenv("GOV2_TOKEN_TTL", "45m")
	t.Setenv("GOV2_TOKEN_ISSUER", "gov2-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.ReadTimeout != 7*time.Second {
		t.Fatalf("expected read timeout override, got %s", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 11*time.Second {
		t.Fatalf("expected write timeout override, got %s", cfg.Server.WriteTimeout)
	}
	if cfg.Server.IdleTimeout != 90*time.Second {
		t.Fatalf("expected idle timeout override, got %s", cfg.Server.IdleTimeout)
	}
	if cfg.Security.TokenTTL != 45*time.Minute {
		t.Fatalf("expected token TTL override, got %s", cfg.Security.TokenTTL)
	}
	if cfg.Security.Issuer != "gov2-test" {
		t.Fatalf("expected token issuer override, got %q", cfg.Security.Issuer)
	}
}

func TestLoadRejectsInvalidDurationOverride(t *testing.T) {
	t.Setenv("GOV2_CONFIG", t.TempDir()+"/missing.json")
	t.Setenv("GOV2_TOKEN_TTL", "forty five minutes")

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid duration override error")
	}
}

func TestLoadAppliesCORSOriginsOverride(t *testing.T) {
	t.Setenv("GOV2_CONFIG", t.TempDir()+"/missing.json")
	t.Setenv("GOV2_CORS_ALLOWED_ORIGINS", "https://admin.gov2.local, https://ops.gov2.local ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []string{"https://admin.gov2.local", "https://ops.gov2.local"}
	if len(cfg.Security.AllowedOrigins) != len(want) {
		t.Fatalf("expected %d origins, got %+v", len(want), cfg.Security.AllowedOrigins)
	}
	for i := range want {
		if cfg.Security.AllowedOrigins[i] != want[i] {
			t.Fatalf("origin[%d]: expected %q, got %q", i, want[i], cfg.Security.AllowedOrigins[i])
		}
	}
}
