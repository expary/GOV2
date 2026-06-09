package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/expary/GOV2/internal/config"
)

func TestVersionTextIncludesBuildInfo(t *testing.T) {
	text := versionText()
	for _, want := range []string{"GOV2 ", "commit:", "build_date:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected version text to contain %q, got %q", want, text)
		}
	}
}

func TestRunModuleScaffoldCreatesFilesWithStableOutput(t *testing.T) {
	output := t.TempDir()
	stdout := captureStdout(t, func() {
		if err := runModuleScaffold([]string{
			"--name", "inventory",
			"--title", "Inventory",
			"--description", "Inventory operations",
			"--output", output,
		}); err != nil {
			t.Fatalf("runModuleScaffold() error = %v", err)
		}
	})

	wantFiles := []string{
		filepath.Join(output, "inventory", "README.md"),
		filepath.Join(output, "inventory", "backend", "module.go"),
		filepath.Join(output, "inventory", "backend", "module_test.go"),
		filepath.Join(output, "inventory", "frontend", "InventoryView.vue"),
		filepath.Join(output, "inventory", "migrations", "000001_init.up.sql"),
		filepath.Join(output, "inventory", "migrations", "000001_init.down.sql"),
	}
	var want bytes.Buffer
	for _, path := range wantFiles {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected scaffold file %s: %v", path, err)
		}
		want.WriteString("created ")
		want.WriteString(path)
		want.WriteByte('\n')
	}
	if stdout != want.String() {
		t.Fatalf("unexpected scaffold output:\n%s\nwant:\n%s", stdout, want.String())
	}
}

func TestCommandDispatchRejectsEmptyArgs(t *testing.T) {
	for name, run := range map[string]func() error{
		"root":   func() error { return runCommand(nil) },
		"module": func() error { return runModule(nil) },
		"admin":  func() error { return runAdmin(nil) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil || !strings.Contains(err.Error(), "usage: gov2") {
				t.Fatalf("expected usage error, got %v", err)
			}
		})
	}
}

func TestOpenSQLDatabaseRejectsMemoryDriverAfterNormalization(t *testing.T) {
	for _, driver := range []string{"", "memory", "Memory", " MEMORY "} {
		t.Run(driver, func(t *testing.T) {
			_, err := openSQLDatabase(config.Config{
				Storage: config.StorageConfig{
					Driver: driver,
					DSN:    "postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable",
				},
			})
			if err == nil || !strings.Contains(err.Error(), "storage.driver") {
				t.Fatalf("expected storage.driver error for %q, got %v", driver, err)
			}
		})
	}
}

func TestOpenSQLDatabaseRejectsBlankDSNAfterNormalization(t *testing.T) {
	_, err := openSQLDatabase(config.Config{
		Storage: config.StorageConfig{
			Driver: " pgx ",
			DSN:    " \t ",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "storage.dsn") {
		t.Fatalf("expected storage.dsn error, got %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = original
	})

	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout pipe writer: %v", err)
	}
	var out bytes.Buffer
	if _, err := io.Copy(&out, reader); err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout pipe reader: %v", err)
	}
	os.Stdout = original
	return out.String()
}
