package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/expary/GOV2/internal/config"
	"github.com/expary/GOV2/internal/httpapi"
	"github.com/expary/GOV2/internal/migration"
	"github.com/expary/GOV2/internal/module"
	"github.com/expary/GOV2/internal/repository"
	"github.com/expary/GOV2/internal/security"
	"github.com/expary/GOV2/internal/service"
	"github.com/expary/GOV2/internal/store/memory"
	"github.com/expary/GOV2/internal/store/sqlstore"
)

type Application struct {
	cfg     config.Config
	logger  *slog.Logger
	handler http.Handler
	close   func() error
}

func New() (config.Config, *Application, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, nil, err
	}
	if err := validateConfig(cfg); err != nil {
		return config.Config{}, nil, err
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if cfg.Security.UsingDefaultSecret {
		logger.Warn("using default token secret; set GOV2_TOKEN_SECRET before production")
	}

	modules, err := module.NewValidatedRegistry(module.BuiltInModules()...)
	if err != nil {
		return config.Config{}, nil, fmt.Errorf("validate built-in modules: %w", err)
	}

	store, closeStore, err := openStore(cfg, logger)
	if err != nil {
		return config.Config{}, nil, err
	}

	tokens := security.NewTokenManager(
		cfg.Security.TokenSecret,
		cfg.Security.TokenTTL,
		cfg.Security.Issuer,
	)
	services := service.NewRegistry(store, tokens, modules.Permissions())
	api := httpapi.New(httpapi.Options{
		Config:   cfg,
		Logger:   logger,
		Modules:  modules,
		Store:    store,
		Services: services,
		Tokens:   tokens,
	})

	return cfg, &Application{
		cfg:     cfg,
		logger:  logger,
		handler: api.Router(),
		close:   closeStore,
	}, nil
}

func (a *Application) Handler() http.Handler {
	return a.handler
}

func (a *Application) Logger() *slog.Logger {
	return a.logger
}

func (a *Application) Close() error {
	if a.close == nil {
		return nil
	}
	return a.close()
}

func validateConfig(cfg config.Config) error {
	if !isProductionEnvironment(cfg.App.Environment) {
		return nil
	}
	if unsafeProductionTokenSecret(cfg.Security) {
		return fmt.Errorf("production token secret must be set to a non-placeholder value with at least 32 characters")
	}
	if isMemoryStorageDriver(cfg.Storage.Driver) {
		return fmt.Errorf("production storage.driver must not be memory")
	}
	if unsafeProductionCORS(cfg.Security) {
		return fmt.Errorf("production CORS allowed origins must not include wildcard")
	}
	return nil
}

func unsafeProductionTokenSecret(security config.SecurityConfig) bool {
	secret := strings.TrimSpace(security.TokenSecret)
	if security.UsingDefaultSecret || secret == "" || len(secret) < 32 {
		return true
	}
	switch secret {
	case "gov2-development-secret-change-me", "change-me-before-production":
		return true
	default:
		return false
	}
}

func unsafeProductionCORS(security config.SecurityConfig) bool {
	for _, origin := range security.AllowedOrigins {
		if strings.TrimSpace(origin) == "*" {
			return true
		}
	}
	return false
}

func isProductionEnvironment(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "production")
}

func storageDriver(value string) string {
	return strings.TrimSpace(value)
}

func isMemoryStorageDriver(value string) bool {
	return strings.EqualFold(storageDriver(value), "memory")
}

func openStore(cfg config.Config, logger *slog.Logger) (repository.Store, func() error, error) {
	driver := storageDriver(cfg.Storage.Driver)
	if isMemoryStorageDriver(driver) {
		store := memory.NewStore()
		if err := store.Seed(); err != nil {
			return nil, nil, err
		}
		return store, nil, nil
	}
	dsn := strings.TrimSpace(cfg.Storage.DSN)
	if dsn == "" {
		return nil, nil, fmt.Errorf("storage.dsn is required for storage driver %q", driver)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	if cfg.Storage.AutoMigrate {
		runner := migration.NewRunner(db, cfg.Storage.MigrationsDir)
		applied, err := runner.RunUp(ctx)
		if err != nil {
			_ = db.Close()
			return nil, nil, err
		}
		logger.Info("database migrations checked", "applied", len(applied))
		if !isProductionEnvironment(cfg.App.Environment) {
			seeds, err := runner.RunSeeds(ctx, cfg.Storage.SeedsDir)
			if err != nil {
				_ = db.Close()
				return nil, nil, err
			}
			logger.Info("database seeds checked", "applied", len(seeds))
		}
	}

	store := sqlstore.New(db)
	if !isProductionEnvironment(cfg.App.Environment) {
		if err := store.BootstrapDevelopmentData(); err != nil {
			_ = db.Close()
			return nil, nil, err
		}
	}
	return store, db.Close, nil
}
