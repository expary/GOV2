package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/expary/GOV2/internal/app"
	"github.com/expary/GOV2/internal/bootstrap"
	"github.com/expary/GOV2/internal/config"
	"github.com/expary/GOV2/internal/migration"
	"github.com/expary/GOV2/internal/scaffold"
	_ "github.com/expary/GOV2/internal/store/postgres"
	"github.com/expary/GOV2/internal/store/sqlstore"
)

var (
	version   = "dev"
	commit    = "local"
	buildDate = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		if err := runCommand(os.Args[1:]); err != nil {
			slog.Error("command failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if err := runServer(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func runServer() error {
	cfg, application, err := app.New()
	if err != nil {
		return fmt.Errorf("create application: %w", err)
	}

	server := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      application.Handler(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	errs := make(chan error, 1)
	go func() {
		application.Logger().Info("gov2 server started", "addr", cfg.Server.Addr)
		errs <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		application.Logger().Info("shutdown signal received", "signal", sig.String())
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server stopped unexpectedly: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	if err := application.Close(); err != nil {
		return fmt.Errorf("close application: %w", err)
	}
	application.Logger().Info("gov2 server stopped")
	return nil
}

func runCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gov2 serve|version|migrate|module|admin")
	}
	switch args[0] {
	case "admin":
		if len(args) < 2 {
			return errors.New("usage: gov2 admin create|reset-password")
		}
		return runAdmin(args[1:])
	case "migrate":
		if len(args) < 2 {
			return errors.New("usage: gov2 migrate up|seed")
		}
		return runMigrate(args[1])
	case "module":
		if len(args) < 2 {
			return errors.New("usage: gov2 module scaffold")
		}
		return runModule(args[1:])
	case "serve":
		return runServer()
	case "version":
		return runVersion()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runVersion() error {
	fmt.Fprintln(os.Stdout, versionText())
	return nil
}

func versionText() string {
	return fmt.Sprintf("GOV2 %s\ncommit: %s\nbuild_date: %s", version, commit, buildDate)
}

func runModule(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gov2 module scaffold")
	}
	switch args[0] {
	case "scaffold":
		return runModuleScaffold(args[1:])
	default:
		return errors.New("usage: gov2 module scaffold")
	}
}

func runModuleScaffold(args []string) error {
	flags := flag.NewFlagSet("gov2 module scaffold", flag.ContinueOnError)
	name := flags.String("name", "", "module name, for example inventory")
	title := flags.String("title", "", "module title")
	description := flags.String("description", "", "module description")
	outputDir := flags.String("output", "modules", "module scaffold output directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	written, err := scaffold.ScaffoldModule(scaffold.ModuleInput{
		Name:        *name,
		Title:       *title,
		Description: *description,
		OutputDir:   *outputDir,
	})
	if err != nil {
		return err
	}
	for _, path := range written {
		fmt.Fprintf(os.Stdout, "created %s\n", path)
	}
	return nil
}

func runAdmin(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gov2 admin create|reset-password")
	}
	switch args[0] {
	case "create":
		return runCreateAdmin(args[1:])
	case "reset-password":
		return runResetAdminPassword(args[1:])
	default:
		return errors.New("usage: gov2 admin create|reset-password")
	}
}

func runCreateAdmin(args []string) error {
	flags := flag.NewFlagSet("gov2 admin create", flag.ContinueOnError)
	username := flags.String("username", getenv("GOV2_ADMIN_USERNAME", "admin"), "administrator username")
	password := flags.String("password", os.Getenv("GOV2_ADMIN_PASSWORD"), "administrator password")
	nickname := flags.String("nickname", os.Getenv("GOV2_ADMIN_NICKNAME"), "administrator nickname")
	email := flags.String("email", os.Getenv("GOV2_ADMIN_EMAIL"), "administrator email")
	phone := flags.String("phone", os.Getenv("GOV2_ADMIN_PHONE"), "administrator phone")
	avatar := flags.String("avatar", os.Getenv("GOV2_ADMIN_AVATAR"), "administrator avatar")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *password == "" {
		return errors.New("administrator password is required; set --password or GOV2_ADMIN_PASSWORD")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openSQLDatabase(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	store := sqlstore.New(db)
	user, err := bootstrap.CreateInitialAdmin(store, bootstrap.AdminInput{
		Username: *username,
		Password: *password,
		Nickname: *nickname,
		Email:    *email,
		Phone:    *phone,
		Avatar:   *avatar,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "created administrator %q with id %d\n", user.Username, user.ID)
	return nil
}

func runResetAdminPassword(args []string) error {
	flags := flag.NewFlagSet("gov2 admin reset-password", flag.ContinueOnError)
	username := flags.String("username", getenv("GOV2_ADMIN_USERNAME", "admin"), "administrator username")
	password := flags.String("password", os.Getenv("GOV2_ADMIN_PASSWORD"), "new administrator password")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *password == "" {
		return errors.New("administrator password is required; set --password or GOV2_ADMIN_PASSWORD")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openSQLDatabase(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	store := sqlstore.New(db)
	user, err := bootstrap.ResetAdminPassword(store, bootstrap.AdminPasswordInput{
		Username: *username,
		Password: *password,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "reset administrator password for %q with id %d\n", user.Username, user.ID)
	return nil
}

func runMigrate(action string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openSQLDatabase(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	return runMigrateWithDB(db, cfg, action)
}

func openSQLDatabase(cfg config.Config) (*sql.DB, error) {
	driver := strings.TrimSpace(cfg.Storage.Driver)
	if driver == "" || strings.EqualFold(driver, "memory") {
		return nil, errors.New("storage.driver must be a registered database/sql driver")
	}
	dsn := strings.TrimSpace(cfg.Storage.DSN)
	if dsn == "" {
		return nil, errors.New("storage.dsn is required")
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func runMigrateWithDB(db *sql.DB, cfg config.Config, action string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runner := migration.NewRunner(db, cfg.Storage.MigrationsDir)
	switch action {
	case "up":
		applied, err := runner.RunUp(ctx)
		if err != nil {
			return err
		}
		for _, item := range applied {
			slog.Info("migration applied", "version", item.Version, "name", item.Name)
		}
		if len(applied) == 0 {
			slog.Info("no pending migrations")
		}
	case "seed":
		applied, err := runner.RunSeeds(ctx, cfg.Storage.SeedsDir)
		if err != nil {
			return err
		}
		for _, name := range applied {
			slog.Info("seed applied", "name", name)
		}
		if len(applied) == 0 {
			slog.Info("no seed files found")
		}
	default:
		return errors.New("usage: gov2 migrate up|seed")
	}
	return nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
