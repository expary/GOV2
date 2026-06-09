package scaffold

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/expary/GOV2/internal/module"
)

var reservedModuleNames = map[string]bool{
	"app":          true,
	"auth":         true,
	"audit":        true,
	"dashboard":    true,
	"dictionaries": true,
	"menus":        true,
	"roles":        true,
	"settings":     true,
	"system":       true,
	"users":        true,
}

type ModuleInput struct {
	Name        string
	Title       string
	Description string
	OutputDir   string
}

type moduleTemplateData struct {
	Name        string
	Title       string
	Description string
	ViewName    string
}

func ScaffoldModule(input ModuleInput) ([]string, error) {
	data, err := normalizeModuleInput(input)
	if err != nil {
		return nil, err
	}
	outputDir := strings.TrimSpace(input.OutputDir)
	if outputDir == "" {
		outputDir = "modules"
	}

	root := filepath.Join(outputDir, data.Name)
	files := []moduleFile{
		{Path: filepath.Join(root, "README.md"), Template: moduleReadmeTemplate},
		{Path: filepath.Join(root, "backend", "module.go"), Template: backendModuleTemplate},
		{Path: filepath.Join(root, "backend", "module_test.go"), Template: backendModuleTestTemplate},
		{Path: filepath.Join(root, "frontend", data.ViewName+".vue"), Template: frontendViewTemplate},
		{Path: filepath.Join(root, "migrations", "000001_init.up.sql"), Template: migrationUpTemplate},
		{Path: filepath.Join(root, "migrations", "000001_init.down.sql"), Template: migrationDownTemplate},
	}
	if err := ensureFilesDoNotExist(files); err != nil {
		return nil, err
	}

	written := make([]string, 0, len(files))
	for _, file := range files {
		content, err := render(file.Template, data)
		if err != nil {
			return nil, err
		}
		if err := writeNewFile(file.Path, content); err != nil {
			return nil, err
		}
		written = append(written, file.Path)
	}
	return written, nil
}

type moduleFile struct {
	Path     string
	Template string
}

func ensureFilesDoNotExist(files []moduleFile) error {
	for _, file := range files {
		if _, err := os.Stat(file.Path); err == nil {
			return fmt.Errorf("file already exists: %s", file.Path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func normalizeModuleInput(input ModuleInput) (moduleTemplateData, error) {
	name := strings.TrimSpace(input.Name)
	if !module.IsValidName(name) {
		return moduleTemplateData{}, errors.New("module name must match " + module.ModuleNamePattern)
	}
	if reservedModuleNames[name] {
		return moduleTemplateData{}, fmt.Errorf("module name %q is reserved for GOV2 built-in modules and API namespaces", name)
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = titleFromName(name)
	}
	description := strings.TrimSpace(input.Description)
	if description == "" {
		description = title + " module"
	}
	return moduleTemplateData{
		Name:        name,
		Title:       title,
		Description: description,
		ViewName:    pascalCase(name) + "View",
	}, nil
}

func render(tmpl string, data moduleTemplateData) (string, error) {
	parsed, err := template.New("module").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := parsed.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func writeNewFile(path string, content string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func titleFromName(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func pascalCase(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

const moduleReadmeTemplate = `# {{.Title}}

{{.Description}}

## Generated Files

- backend/module.go: module metadata, permission definitions, backend route metadata, menu entry, frontend route, and migration reference.
- backend/module_test.go: metadata validation test for the generated module contract.
- frontend/{{.ViewName}}.vue: starter Vue view for the module.
- migrations/000001_init.up.sql: starter migration.
- migrations/000001_init.down.sql: starter rollback migration.

## Integration Checklist

1. Register the backend module in application startup.
2. Add backend routes and repository/service contracts.
3. Add frontend routes and navigation metadata.
4. Run ` + "`go test ./modules/{{.Name}}/backend`" + ` after editing metadata.
5. Add service, repository, route, and frontend tests before wiring the module into production menus.
`

const backendModuleTemplate = `package backend

import (
	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/module"
)

const (
	Name = {{printf "%q" .Name}}

	PermissionList   = "{{.Name}}:item:list"
	PermissionCreate = "{{.Name}}:item:create"
	PermissionUpdate = "{{.Name}}:item:update"
	PermissionDelete = "{{.Name}}:item:delete"
)

func Module() module.Module {
	return module.Module{
		Name:        Name,
		Title:       {{printf "%q" .Title}},
		Version:     "0.1.0",
		Description: {{printf "%q" .Description}},
		Permissions: []domain.PermissionDefinition{
			{Code: PermissionList, Name: {{printf "%q" (printf "List %s items" .Title)}}, Module: Name, Description: {{printf "%q" (printf "View %s items" .Title)}}},
			{Code: PermissionCreate, Name: {{printf "%q" (printf "Create %s items" .Title)}}, Module: Name, Description: {{printf "%q" (printf "Create %s items" .Title)}}},
			{Code: PermissionUpdate, Name: {{printf "%q" (printf "Update %s items" .Title)}}, Module: Name, Description: {{printf "%q" (printf "Update %s items" .Title)}}},
			{Code: PermissionDelete, Name: {{printf "%q" (printf "Delete %s items" .Title)}}, Module: Name, Description: {{printf "%q" (printf "Delete %s items" .Title)}}},
		},
		Backend: []module.BackendRoute{
			{Name: "{{.Name}}-items-list", Method: "GET", Path: "/api/v1/{{.Name}}/items", Permission: PermissionList, Summary: {{printf "%q" (printf "List %s items" .Title)}}},
			{Name: "{{.Name}}-items-create", Method: "POST", Path: "/api/v1/{{.Name}}/items", Permission: PermissionCreate, Summary: {{printf "%q" (printf "Create %s item" .Title)}}},
			{Name: "{{.Name}}-items-update", Method: "PUT", Path: "/api/v1/{{.Name}}/items/{id}", Permission: PermissionUpdate, Summary: {{printf "%q" (printf "Update %s item" .Title)}}},
			{Name: "{{.Name}}-items-delete", Method: "DELETE", Path: "/api/v1/{{.Name}}/items/{id}", Permission: PermissionDelete, Summary: {{printf "%q" (printf "Delete %s item" .Title)}}},
		},
		Menus: []module.MenuEntry{
			{Name: Name, Title: {{printf "%q" .Title}}, Path: "/{{.Name}}", Icon: "box", Component: {{printf "%q" .ViewName}}, Permission: PermissionList, Sort: 1000},
		},
		Routes: []module.FrontendRoute{
			{Name: Name, Path: "/{{.Name}}", Component: {{printf "%q" .ViewName}}, Title: {{printf "%q" .Title}}, Permission: PermissionList},
		},
		Migrations: []module.MigrationSet{
			{Driver: "postgres", Dir: "modules/{{.Name}}/migrations"},
		},
	}
}
`

const backendModuleTestTemplate = `package backend

import (
	"testing"

	"github.com/expary/GOV2/internal/module"
)

func TestModuleMetadataIsValid(t *testing.T) {
	if err := module.ValidateModules(Module()); err != nil {
		t.Fatalf("module metadata is invalid: %v", err)
	}
}
`

const frontendViewTemplate = `<script setup>
const title = {{printf "%q" .Title}};
</script>

<template>
  <section class="panel">
    <header>
      <h1>{{ "{{ title }}" }}</h1>
    </header>
  </section>
</template>
`

const migrationUpTemplate = `-- Add {{.Name}} module tables here.
`

const migrationDownTemplate = `-- Drop {{.Name}} module tables here.
`
