package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldModuleWritesExpectedFiles(t *testing.T) {
	output := t.TempDir()

	written, err := ScaffoldModule(ModuleInput{
		Name:        "inventory",
		Title:       "Inventory",
		Description: "Inventory operations",
		OutputDir:   output,
	})
	if err != nil {
		t.Fatalf("ScaffoldModule() error = %v", err)
	}

	wantFiles := []string{
		filepath.Join(output, "inventory", "README.md"),
		filepath.Join(output, "inventory", "backend", "module.go"),
		filepath.Join(output, "inventory", "backend", "module_test.go"),
		filepath.Join(output, "inventory", "frontend", "InventoryView.vue"),
		filepath.Join(output, "inventory", "migrations", "000001_init.up.sql"),
		filepath.Join(output, "inventory", "migrations", "000001_init.down.sql"),
	}
	if len(written) != len(wantFiles) {
		t.Fatalf("expected %d written files, got %+v", len(wantFiles), written)
	}
	for i, path := range wantFiles {
		if written[i] != path {
			t.Fatalf("written[%d] = %s, want %s", i, written[i], path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated file %s: %v", path, err)
		}
	}

	backend, err := os.ReadFile(filepath.Join(output, "inventory", "backend", "module.go"))
	if err != nil {
		t.Fatalf("read backend module: %v", err)
	}
	for _, want := range []string{
		`Name = "inventory"`,
		`PermissionList   = "inventory:item:list"`,
		`Title:       "Inventory"`,
		`Backend: []module.BackendRoute`,
		`Path: "/api/v1/inventory/items"`,
		`Menus: []module.MenuEntry`,
		`Routes: []module.FrontendRoute`,
		`Migrations: []module.MigrationSet`,
		`Path: "/inventory"`,
		`Dir: "modules/inventory/migrations"`,
	} {
		if !strings.Contains(string(backend), want) {
			t.Fatalf("expected backend module to contain %q, got %s", want, backend)
		}
	}

	backendTest, err := os.ReadFile(filepath.Join(output, "inventory", "backend", "module_test.go"))
	if err != nil {
		t.Fatalf("read backend module test: %v", err)
	}
	for _, want := range []string{
		`func TestModuleMetadataIsValid(t *testing.T)`,
		`module.ValidateModules(Module())`,
	} {
		if !strings.Contains(string(backendTest), want) {
			t.Fatalf("expected backend module test to contain %q, got %s", want, backendTest)
		}
	}

	readme, err := os.ReadFile(filepath.Join(output, "inventory", "README.md"))
	if err != nil {
		t.Fatalf("read module README: %v", err)
	}
	for _, want := range []string{
		`backend/module_test.go: metadata validation test`,
		"go test ./modules/inventory/backend",
	} {
		if !strings.Contains(string(readme), want) {
			t.Fatalf("expected README to contain %q, got %s", want, readme)
		}
	}

	view, err := os.ReadFile(filepath.Join(output, "inventory", "frontend", "InventoryView.vue"))
	if err != nil {
		t.Fatalf("read frontend view: %v", err)
	}
	if !strings.Contains(string(view), "{{ title }}") {
		t.Fatalf("expected Vue title binding, got %s", view)
	}
	if !strings.Contains(string(view), `class="panel"`) {
		t.Fatalf("expected starter view to use existing panel class, got %s", view)
	}
}

func TestScaffoldModuleRejectsInvalidName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "Bad-Name", want: "module name must match"},
		{name: "system", want: `module name "system" is reserved`},
		{name: "dashboard", want: `module name "dashboard" is reserved`},
		{name: "auth", want: `module name "auth" is reserved`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ScaffoldModule(ModuleInput{Name: tt.name, OutputDir: t.TempDir()})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected module name error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestScaffoldModuleRefusesOverwrite(t *testing.T) {
	output := t.TempDir()
	input := ModuleInput{Name: "inventory", OutputDir: output}
	if _, err := ScaffoldModule(input); err != nil {
		t.Fatalf("first ScaffoldModule() error = %v", err)
	}
	if _, err := ScaffoldModule(input); err == nil || !strings.Contains(err.Error(), "file already exists") {
		t.Fatalf("expected overwrite error, got %v", err)
	}
}

func TestScaffoldModuleGeneratedBackendPackagePassesGoTest(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	scratchRoot := filepath.Join(repoRoot, "internal", "scaffold", "testdata")
	if err := os.MkdirAll(scratchRoot, 0o755); err != nil {
		t.Fatalf("create scaffold testdata dir: %v", err)
	}
	output, err := os.MkdirTemp(scratchRoot, "scaffold-")
	if err != nil {
		t.Fatalf("create scaffold temp output dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(output); err != nil {
			t.Errorf("remove scaffold temp output dir: %v", err)
		}
		_ = os.Remove(scratchRoot)
	})

	if _, err := ScaffoldModule(ModuleInput{
		Name:        "inventory",
		Title:       "Inventory",
		Description: "Inventory operations",
		OutputDir:   output,
	}); err != nil {
		t.Fatalf("ScaffoldModule() error = %v", err)
	}

	backendDir := filepath.Join(output, "inventory", "backend")
	relBackendDir, err := filepath.Rel(repoRoot, backendDir)
	if err != nil {
		t.Fatalf("resolve generated backend package path: %v", err)
	}
	pkg := "./" + filepath.ToSlash(relBackendDir)
	cmd := exec.Command("go", "test", pkg, "-count=1")
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated backend package failed go test %s: %v\n%s", pkg, err, out)
	}
}

func TestScaffoldModulePreflightsExistingTargets(t *testing.T) {
	output := t.TempDir()
	existing := filepath.Join(output, "inventory", "migrations", "000001_init.up.sql")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatalf("create existing target parent: %v", err)
	}
	if err := os.WriteFile(existing, []byte("-- existing\n"), 0o644); err != nil {
		t.Fatalf("create existing target: %v", err)
	}

	written, err := ScaffoldModule(ModuleInput{Name: "inventory", OutputDir: output})
	if err == nil || !strings.Contains(err.Error(), "file already exists") {
		t.Fatalf("expected existing target error, got written=%+v err=%v", written, err)
	}
	for _, path := range []string{
		filepath.Join(output, "inventory", "README.md"),
		filepath.Join(output, "inventory", "backend", "module.go"),
		filepath.Join(output, "inventory", "backend", "module_test.go"),
		filepath.Join(output, "inventory", "frontend", "InventoryView.vue"),
		filepath.Join(output, "inventory", "migrations", "000001_init.down.sql"),
	} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("unexpected partial scaffold file %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat partial scaffold file %s: %v", path, err)
		}
	}
}
