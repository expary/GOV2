package migration

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/module"
)

func TestSystemSeedDoesNotOverwriteSiteTitleSetting(t *testing.T) {
	sql := readSystemSeed(t)
	siteTitleIndex := strings.Index(sql, "'site.title'")
	if siteTitleIndex == -1 {
		t.Fatal("system seed must create the site.title setting")
	}
	siteTitleSeed := sql[siteTitleIndex:]
	if !strings.Contains(siteTitleSeed, `ON CONFLICT (lower("key")) DO NOTHING`) {
		t.Fatalf("site.title seed must not overwrite an existing operational setting, got:\n%s", siteTitleSeed)
	}
	if strings.Contains(siteTitleSeed, "DO UPDATE") {
		t.Fatalf("site.title seed must not use DO UPDATE, got:\n%s", siteTitleSeed)
	}
}

func TestSystemSeedPermissionsMatchDomainRegistry(t *testing.T) {
	got := seedPermissions(t, readSystemSeed(t))
	want := map[string]seedPermission{
		domain.PermissionAll: {
			Code:        domain.PermissionAll,
			Name:        "All permissions",
			Module:      "system",
			Description: "Full system access",
		},
	}
	for _, permission := range domain.SystemPermissions() {
		want[permission.Code] = seedPermission{
			Code:        permission.Code,
			Name:        permission.Name,
			Module:      permission.Module,
			Description: permission.Description,
		}
	}

	assertMapKeys(t, "seed permissions", stringKeys(got), stringKeys(want))
	for code, wantPermission := range want {
		if gotPermission := got[code]; gotPermission != wantPermission {
			t.Fatalf("seed permission %q = %+v, want %+v", code, gotPermission, wantPermission)
		}
	}
}

func TestSystemSeedDoesNotCreateDefaultUsers(t *testing.T) {
	sql := strings.ToLower(readSystemSeed(t))
	for _, table := range []string{"gov2_users", "gov2_user_roles"} {
		if strings.Contains(sql, "insert into "+table) {
			t.Fatalf("system seed must not create default production user data in %s", table)
		}
	}
}

func TestSystemSeedRolePermissionsMatchBuiltInDefaults(t *testing.T) {
	got := seedRolePermissions(t, readSystemSeed(t))
	want := map[string][]string{
		"admin":    {domain.PermissionAll},
		"operator": sortedStrings(operatorDefaultPermissions()),
	}

	assertMapKeys(t, "seed role permissions", stringKeys(got), stringKeys(want))
	for code, wantPermissions := range want {
		if strings.Join(got[code], "\n") != strings.Join(wantPermissions, "\n") {
			t.Fatalf("seed role %q permissions = %+v, want %+v", code, got[code], wantPermissions)
		}
	}
}

func TestSystemSeedMenusMatchBuiltInModuleRegistry(t *testing.T) {
	got := seedMenus(t, readSystemSeed(t))
	want := map[string]seedMenu{}
	registry := module.NewRegistry(module.BuiltInModules()...)
	for _, menu := range registry.Menus() {
		want[menu.Name] = seedMenu{
			Name:       menu.Name,
			Parent:     menu.Parent,
			Title:      menu.Title,
			Path:       menu.Path,
			Icon:       menu.Icon,
			Component:  menu.Component,
			Permission: menu.Permission,
			Sort:       menu.Sort,
			Hidden:     menu.Hidden,
		}
	}

	assertMapKeys(t, "seed menus", stringKeys(got), stringKeys(want))
	for name, wantMenu := range want {
		if gotMenu := got[name]; gotMenu != wantMenu {
			t.Fatalf("seed menu %q = %+v, want %+v", name, gotMenu, wantMenu)
		}
	}
}

func TestSystemSeedUserStatusDictionaryMatchesDomain(t *testing.T) {
	sql := readSystemSeed(t)
	dictionary := seedDictionary(t, sql, "user_status")
	if dictionary != (seedDictionaryDefinition{
		Code:        "user_status",
		Name:        "User Status",
		Description: "Built-in user status dictionary",
	}) {
		t.Fatalf("user_status seed dictionary = %+v", dictionary)
	}

	got := seedDictionaryItems(t, sql, "user_status")
	want := map[string]seedDictionaryItem{
		domain.UserStatusActive:   {Label: "Active", Value: domain.UserStatusActive, Sort: 1},
		domain.UserStatusDisabled: {Label: "Disabled", Value: domain.UserStatusDisabled, Sort: 2},
	}

	assertMapKeys(t, "seed user_status items", stringKeys(got), stringKeys(want))
	for value, wantItem := range want {
		if gotItem := got[value]; gotItem != wantItem {
			t.Fatalf("user_status item %q = %+v, want %+v", value, gotItem, wantItem)
		}
	}
}

func TestMigrationsDefineAuditLogFilterIndexes(t *testing.T) {
	sql := strings.ToLower(readUpMigrations(t))
	requiredIndexes := []string{
		"create index gov2_audit_logs_created_at_idx on gov2_audit_logs (created_at desc)",
		"create index gov2_audit_logs_actor_created_idx on gov2_audit_logs (actor_id, created_at desc)",
		"create index gov2_audit_logs_action_created_idx on gov2_audit_logs (action, created_at desc)",
		"create index gov2_audit_logs_resource_created_idx on gov2_audit_logs (resource, created_at desc)",
		"create index gov2_audit_logs_resource_id_created_idx on gov2_audit_logs (resource_id, created_at desc)",
	}
	for _, index := range requiredIndexes {
		if !strings.Contains(sql, index) {
			t.Fatalf("migrations must define audit-log filter index: %s", index)
		}
	}
}

func TestMigrationFilesAreNumberedAndPaired(t *testing.T) {
	files := migrationFileNames(t)
	namePattern := regexp.MustCompile(`^([0-9]{6}_[a-z0-9_]+)\.(up|down)\.sql$`)
	up := map[string]struct{}{}
	down := map[string]struct{}{}

	for _, name := range files {
		match := namePattern.FindStringSubmatch(name)
		if match == nil {
			t.Fatalf("migration file %q must match NNNNNN_name.(up|down).sql", name)
		}
		switch match[2] {
		case "up":
			up[match[1]] = struct{}{}
		case "down":
			down[match[1]] = struct{}{}
		}
	}

	assertMapKeys(t, "migration up/down pairs", stringKeys(up), stringKeys(down))
}

func readSystemSeed(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile("../../migrations/seeds/system.sql")
	if err != nil {
		t.Fatalf("read system seed: %v", err)
	}
	return string(data)
}

func readUpMigrations(t *testing.T) string {
	t.Helper()

	dir := "../../migrations"
	names := make([]string, 0)
	for _, name := range migrationFileNames(t) {
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		parts = append(parts, string(data))
	}
	return strings.Join(parts, "\n")
}

func migrationFileNames(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

type seedPermission struct {
	Code        string
	Name        string
	Module      string
	Description string
}

func seedPermissions(t *testing.T, sql string) map[string]seedPermission {
	t.Helper()

	section := sectionBetween(t, sql, "INSERT INTO gov2_permissions", "ON CONFLICT (code)")
	tuplePattern := regexp.MustCompile(`\(\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)'\s*\)`)
	permissions := map[string]seedPermission{}
	for _, match := range tuplePattern.FindAllStringSubmatch(section, -1) {
		permissions[match[1]] = seedPermission{
			Code:        match[1],
			Name:        match[2],
			Module:      match[3],
			Description: match[4],
		}
	}
	if len(permissions) == 0 {
		t.Fatal("system seed must declare permission tuples")
	}
	return permissions
}

func seedRolePermissions(t *testing.T, sql string) map[string][]string {
	t.Helper()

	roles := map[string][]string{}

	singlePattern := regexp.MustCompile(`(?s)INSERT INTO gov2_role_permissions.*?JOIN gov2_permissions p ON p\.code = '([^']*)'.*?WHERE r\.code = '([^']*)'`)
	for _, match := range singlePattern.FindAllStringSubmatch(sql, -1) {
		roles[match[2]] = append(roles[match[2]], match[1])
	}

	listPattern := regexp.MustCompile(`(?s)INSERT INTO gov2_role_permissions.*?JOIN gov2_permissions p ON p\.code IN \((.*?)\).*?WHERE r\.code = '([^']*)'`)
	for _, match := range listPattern.FindAllStringSubmatch(sql, -1) {
		roles[match[2]] = append(roles[match[2]], quotedSQLValues(match[1])...)
	}

	if len(roles) == 0 {
		t.Fatal("system seed must assign role permissions")
	}
	for code, permissions := range roles {
		roles[code] = sortedStrings(permissions)
	}
	return roles
}

func quotedSQLValues(section string) []string {
	valuePattern := regexp.MustCompile(`'([^']*)'`)
	matches := valuePattern.FindAllStringSubmatch(section, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	return values
}

type seedMenu struct {
	Name       string
	Parent     string
	Title      string
	Path       string
	Icon       string
	Component  string
	Permission string
	Sort       int
	Hidden     bool
}

func seedMenus(t *testing.T, sql string) map[string]seedMenu {
	t.Helper()

	menus := map[string]seedMenu{}
	rootSection := sectionBetween(
		t,
		sql,
		"INSERT INTO gov2_menus (title, name, path, icon, component, permission_code, sort)",
		"ON CONFLICT (lower(name)) WHERE deleted_at IS NULL DO UPDATE SET",
	)
	for name, menu := range parseSeedMenuTuples(t, rootSection, "") {
		menus[name] = menu
	}

	childSection := sectionBetween(
		t,
		sql,
		"INSERT INTO gov2_menus (parent_id, title, name, path, icon, component, permission_code, sort)",
		") AS child",
	)
	for name, menu := range parseSeedMenuTuples(t, childSection, "system") {
		menus[name] = menu
	}
	if len(menus) == 0 {
		t.Fatal("system seed must declare menu tuples")
	}
	return menus
}

func parseSeedMenuTuples(t *testing.T, section string, parent string) map[string]seedMenu {
	t.Helper()

	tuplePattern := regexp.MustCompile(`\(\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*(NULL|'([^']*)'),\s*([0-9]+)\s*\)`)
	menus := map[string]seedMenu{}
	for _, match := range tuplePattern.FindAllStringSubmatch(section, -1) {
		sortValue, err := strconv.Atoi(match[8])
		if err != nil {
			t.Fatalf("parse menu sort %q: %v", match[8], err)
		}
		menu := seedMenu{
			Title:      match[1],
			Name:       match[2],
			Path:       match[3],
			Icon:       match[4],
			Component:  match[5],
			Permission: match[7],
			Sort:       sortValue,
			Parent:     parent,
		}
		menus[menu.Name] = menu
	}
	return menus
}

type seedDictionaryDefinition struct {
	Code        string
	Name        string
	Description string
}

func seedDictionary(t *testing.T, sql string, code string) seedDictionaryDefinition {
	t.Helper()

	section := sectionBetween(t, sql, "INSERT INTO gov2_dictionaries", "ON CONFLICT (lower(code))")
	tuplePattern := regexp.MustCompile(`\(\s*'([^']*)',\s*'([^']*)',\s*'([^']*)'\s*\)`)
	for _, match := range tuplePattern.FindAllStringSubmatch(section, -1) {
		if match[1] == code {
			return seedDictionaryDefinition{
				Code:        match[1],
				Name:        match[2],
				Description: match[3],
			}
		}
	}
	t.Fatalf("system seed must declare dictionary %q", code)
	return seedDictionaryDefinition{}
}

type seedDictionaryItem struct {
	Label string
	Value string
	Sort  int
}

func seedDictionaryItems(t *testing.T, sql string, dictionaryCode string) map[string]seedDictionaryItem {
	t.Helper()

	section := sectionBetween(t, sql, "INSERT INTO gov2_dictionary_items", "ON CONFLICT (dictionary_id, value)")
	if !strings.Contains(section, "ON d.code = '"+dictionaryCode+"'") {
		t.Fatalf("system seed dictionary items must target dictionary %q", dictionaryCode)
	}
	tuplePattern := regexp.MustCompile(`\(\s*'([^']*)',\s*'([^']*)',\s*([0-9]+)\s*\)`)
	items := map[string]seedDictionaryItem{}
	for _, match := range tuplePattern.FindAllStringSubmatch(section, -1) {
		sortValue, err := strconv.Atoi(match[3])
		if err != nil {
			t.Fatalf("parse dictionary item sort %q: %v", match[3], err)
		}
		item := seedDictionaryItem{
			Label: match[1],
			Value: match[2],
			Sort:  sortValue,
		}
		items[item.Value] = item
	}
	if len(items) == 0 {
		t.Fatalf("system seed must declare dictionary items for %q", dictionaryCode)
	}
	return items
}

func operatorDefaultPermissions() []string {
	return []string{
		domain.PermissionDashboardView,
		domain.PermissionSystemUserList,
		domain.PermissionSystemRoleList,
		domain.PermissionSystemMenuList,
		domain.PermissionSystemModuleList,
		domain.PermissionSystemDictionaryList,
		domain.PermissionSystemSettingList,
		domain.PermissionSystemAuditList,
	}
}

func sectionBetween(t *testing.T, source string, start string, end string) string {
	t.Helper()

	startIndex := strings.Index(source, start)
	if startIndex == -1 {
		t.Fatalf("seed SQL missing section start %q", start)
	}
	remainder := source[startIndex:]
	endIndex := strings.Index(remainder, end)
	if endIndex == -1 {
		t.Fatalf("seed SQL section %q missing end %q", start, end)
	}
	return remainder[:endIndex]
}

func assertMapKeys(t *testing.T, label string, got []string, want []string) {
	t.Helper()

	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s mismatch:\ngot:\n%s\nwant:\n%s", label, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func sortedStrings(items []string) []string {
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}

func stringKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	return keys
}
