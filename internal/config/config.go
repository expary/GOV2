package config

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App      AppConfig      `json:"app"`
	Server   ServerConfig   `json:"server"`
	Security SecurityConfig `json:"security"`
	Storage  StorageConfig  `json:"storage"`
	Web      WebConfig      `json:"web"`
}

type AppConfig struct {
	Name        string `json:"name"`
	Environment string `json:"environment"`
}

type ServerConfig struct {
	Addr         string        `json:"addr"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
}

type SecurityConfig struct {
	TokenSecret        string        `json:"token_secret"`
	TokenTTL           time.Duration `json:"token_ttl"`
	Issuer             string        `json:"issuer"`
	AllowedOrigins     []string      `json:"allowed_origins"`
	UsingDefaultSecret bool          `json:"-"`
}

type WebConfig struct {
	StaticDir string `json:"static_dir"`
}

type StorageConfig struct {
	Driver        string `json:"driver"`
	DSN           string `json:"dsn"`
	AutoMigrate   bool   `json:"auto_migrate"`
	MigrationsDir string `json:"migrations_dir"`
	SeedsDir      string `json:"seeds_dir"`
}

func Load() (Config, error) {
	cfg := defaults()
	path := getenv("GOV2_CONFIG", "config/gov2.json")

	if data, err := os.ReadFile(path); err == nil {
		if err := unmarshalConfig(data, &cfg); err != nil {
			return Config{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}

	if v := os.Getenv("GOV2_ENVIRONMENT"); v != "" {
		cfg.App.Environment = v
	}
	if v := os.Getenv("GOV2_APP_NAME"); v != "" {
		cfg.App.Name = v
	}
	if v := os.Getenv("GOV2_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if err := applyDurationEnv("GOV2_SERVER_READ_TIMEOUT", &cfg.Server.ReadTimeout); err != nil {
		return Config{}, err
	}
	if err := applyDurationEnv("GOV2_SERVER_WRITE_TIMEOUT", &cfg.Server.WriteTimeout); err != nil {
		return Config{}, err
	}
	if err := applyDurationEnv("GOV2_SERVER_IDLE_TIMEOUT", &cfg.Server.IdleTimeout); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("GOV2_TOKEN_SECRET"); v != "" {
		cfg.Security.TokenSecret = v
		cfg.Security.UsingDefaultSecret = v == defaults().Security.TokenSecret
	}
	if err := applyDurationEnv("GOV2_TOKEN_TTL", &cfg.Security.TokenTTL); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("GOV2_TOKEN_ISSUER"); v != "" {
		cfg.Security.Issuer = v
	}
	if v := os.Getenv("GOV2_CORS_ALLOWED_ORIGINS"); v != "" {
		cfg.Security.AllowedOrigins = splitCSV(v)
	}
	if v := os.Getenv("GOV2_STATIC_DIR"); v != "" {
		cfg.Web.StaticDir = v
	}
	if v := os.Getenv("GOV2_STORAGE_DRIVER"); v != "" {
		cfg.Storage.Driver = v
	}
	if v := os.Getenv("GOV2_STORAGE_DSN"); v != "" {
		cfg.Storage.DSN = v
	}
	if v := os.Getenv("GOV2_AUTO_MIGRATE"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, err
		}
		cfg.Storage.AutoMigrate = parsed
	}
	if v := os.Getenv("GOV2_MIGRATIONS_DIR"); v != "" {
		cfg.Storage.MigrationsDir = v
	}
	if v := os.Getenv("GOV2_SEEDS_DIR"); v != "" {
		cfg.Storage.SeedsDir = v
	}

	if cfg.Security.TokenSecret == "" {
		cfg.Security.TokenSecret = defaults().Security.TokenSecret
		cfg.Security.UsingDefaultSecret = true
	}
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Web.StaticDir == "" {
		cfg.Web.StaticDir = "web/dist"
	}
	if cfg.Security.Issuer == "" {
		cfg.Security.Issuer = "gov2"
	}
	if cfg.Security.TokenTTL <= 0 {
		cfg.Security.TokenTTL = 2 * time.Hour
	}
	if cfg.Storage.Driver == "" {
		cfg.Storage.Driver = "memory"
	}
	if cfg.Storage.MigrationsDir == "" {
		cfg.Storage.MigrationsDir = "migrations"
	}
	if cfg.Storage.SeedsDir == "" {
		cfg.Storage.SeedsDir = "migrations/seeds"
	}

	return cfg, nil
}

func defaults() Config {
	return Config{
		App: AppConfig{
			Name:        "GOV2",
			Environment: "development",
		},
		Server: ServerConfig{
			Addr:         ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Security: SecurityConfig{
			TokenSecret:        "gov2-development-secret-change-me",
			TokenTTL:           2 * time.Hour,
			Issuer:             "gov2",
			AllowedOrigins:     []string{"*"},
			UsingDefaultSecret: true,
		},
		Storage: StorageConfig{
			Driver:        "memory",
			MigrationsDir: "migrations",
			SeedsDir:      "migrations/seeds",
		},
		Web: WebConfig{
			StaticDir: "web/dist",
		},
	}
}

type rawConfig struct {
	App      AppConfig         `json:"app"`
	Server   rawServerConfig   `json:"server"`
	Security rawSecurityConfig `json:"security"`
	Storage  StorageConfig     `json:"storage"`
	Web      WebConfig         `json:"web"`
}

type rawServerConfig struct {
	Addr         string `json:"addr"`
	ReadTimeout  string `json:"read_timeout"`
	WriteTimeout string `json:"write_timeout"`
	IdleTimeout  string `json:"idle_timeout"`
}

type rawSecurityConfig struct {
	TokenSecret    string   `json:"token_secret"`
	TokenTTL       string   `json:"token_ttl"`
	Issuer         string   `json:"issuer"`
	AllowedOrigins []string `json:"allowed_origins"`
}

func unmarshalConfig(data []byte, cfg *Config) error {
	raw := rawConfig{
		App: cfg.App,
		Server: rawServerConfig{
			Addr:         cfg.Server.Addr,
			ReadTimeout:  cfg.Server.ReadTimeout.String(),
			WriteTimeout: cfg.Server.WriteTimeout.String(),
			IdleTimeout:  cfg.Server.IdleTimeout.String(),
		},
		Security: rawSecurityConfig{
			TokenSecret:    cfg.Security.TokenSecret,
			TokenTTL:       cfg.Security.TokenTTL.String(),
			Issuer:         cfg.Security.Issuer,
			AllowedOrigins: append([]string(nil), cfg.Security.AllowedOrigins...),
		},
		Storage: cfg.Storage,
		Web:     cfg.Web,
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	cfg.App = raw.App
	cfg.Server.Addr = raw.Server.Addr
	cfg.Storage = raw.Storage
	cfg.Web = raw.Web
	cfg.Security.TokenSecret = raw.Security.TokenSecret
	cfg.Security.Issuer = raw.Security.Issuer
	cfg.Security.AllowedOrigins = cleanOrigins(raw.Security.AllowedOrigins)
	cfg.Security.UsingDefaultSecret = raw.Security.TokenSecret == "" || raw.Security.TokenSecret == defaults().Security.TokenSecret

	var err error
	if raw.Server.ReadTimeout != "" {
		cfg.Server.ReadTimeout, err = time.ParseDuration(raw.Server.ReadTimeout)
		if err != nil {
			return err
		}
	}
	if raw.Server.WriteTimeout != "" {
		cfg.Server.WriteTimeout, err = time.ParseDuration(raw.Server.WriteTimeout)
		if err != nil {
			return err
		}
	}
	if raw.Server.IdleTimeout != "" {
		cfg.Server.IdleTimeout, err = time.ParseDuration(raw.Server.IdleTimeout)
		if err != nil {
			return err
		}
	}
	if raw.Security.TokenTTL != "" {
		cfg.Security.TokenTTL, err = time.ParseDuration(raw.Security.TokenTTL)
		if err != nil {
			return err
		}
	}

	return nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	return cleanOrigins(parts)
}

func cleanOrigins(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func applyDurationEnv(key string, target *time.Duration) error {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	*target = parsed
	return nil
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
