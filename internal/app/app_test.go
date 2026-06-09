package app

import (
	"strings"
	"testing"

	"github.com/expary/GOV2/internal/config"
)

func TestValidateConfigAllowsDevelopmentDefaults(t *testing.T) {
	err := validateConfig(config.Config{
		App: config.AppConfig{Environment: "development"},
		Security: config.SecurityConfig{
			TokenSecret:        "gov2-development-secret-change-me",
			UsingDefaultSecret: true,
		},
		Storage: config.StorageConfig{Driver: "memory"},
	})
	if err != nil {
		t.Fatalf("expected development defaults to be allowed, got %v", err)
	}
}

func TestValidateConfigRejectsProductionDefaultSecret(t *testing.T) {
	err := validateConfig(config.Config{
		App: config.AppConfig{Environment: "production"},
		Security: config.SecurityConfig{
			TokenSecret:        "gov2-development-secret-change-me",
			UsingDefaultSecret: true,
		},
		Storage: config.StorageConfig{Driver: "pgx"},
	})
	if err == nil || !strings.Contains(err.Error(), "production token secret") {
		t.Fatalf("expected production token secret error, got %v", err)
	}
}

func TestValidateConfigRejectsProductionMemoryStore(t *testing.T) {
	err := validateConfig(config.Config{
		App: config.AppConfig{Environment: " Production "},
		Security: config.SecurityConfig{
			TokenSecret: "production-secret-with-more-than-32-characters",
		},
		Storage: config.StorageConfig{Driver: " MEMORY "},
	})
	if err == nil || !strings.Contains(err.Error(), "storage.driver") {
		t.Fatalf("expected production memory store error, got %v", err)
	}
}

func TestProductionEnvironmentNormalization(t *testing.T) {
	for _, value := range []string{"production", "Production", " PRODUCTION "} {
		if !isProductionEnvironment(value) {
			t.Fatalf("expected %q to be treated as production", value)
		}
	}
	for _, value := range []string{"development", "prod", ""} {
		if isProductionEnvironment(value) {
			t.Fatalf("expected %q not to be treated as production", value)
		}
	}
}

func TestMemoryStorageDriverNormalization(t *testing.T) {
	for _, value := range []string{"memory", "Memory", " MEMORY "} {
		if !isMemoryStorageDriver(value) {
			t.Fatalf("expected %q to be treated as memory storage", value)
		}
	}
	for _, value := range []string{"pgx", "sqlite", ""} {
		if isMemoryStorageDriver(value) {
			t.Fatalf("expected %q not to be treated as memory storage", value)
		}
	}
}

func TestValidateConfigRejectsProductionWildcardCORS(t *testing.T) {
	err := validateConfig(config.Config{
		App: config.AppConfig{Environment: "production"},
		Security: config.SecurityConfig{
			TokenSecret:    "production-secret-with-more-than-32-characters",
			AllowedOrigins: []string{"*"},
		},
		Storage: config.StorageConfig{Driver: "pgx"},
	})
	if err == nil || !strings.Contains(err.Error(), "CORS") {
		t.Fatalf("expected production CORS error, got %v", err)
	}
}

func TestValidateConfigAllowsProductionSQLStore(t *testing.T) {
	err := validateConfig(config.Config{
		App: config.AppConfig{Environment: "production"},
		Security: config.SecurityConfig{
			TokenSecret: "production-secret-with-more-than-32-characters",
		},
		Storage: config.StorageConfig{Driver: "pgx"},
	})
	if err != nil {
		t.Fatalf("expected production SQL config to be allowed, got %v", err)
	}
}

func TestOpenStoreRejectsBlankSQLDSNAfterNormalization(t *testing.T) {
	_, _, err := openStore(config.Config{
		Storage: config.StorageConfig{
			Driver: " pgx ",
			DSN:    " \t ",
		},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "storage.dsn") {
		t.Fatalf("expected storage.dsn error, got %v", err)
	}
}
