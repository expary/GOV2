INSERT INTO gov2_permissions (code, name, module, description)
VALUES
  ('*', 'All permissions', 'system', 'Full system access'),
  ('dashboard:view', 'View dashboard', 'dashboard', 'View dashboard summary'),
  ('system:user:list', 'List users', 'system', 'View system users'),
  ('system:user:create', 'Create users', 'system', 'Create system users'),
  ('system:user:update', 'Update users', 'system', 'Update system users'),
  ('system:user:delete', 'Delete users', 'system', 'Delete system users'),
  ('system:role:list', 'List roles', 'system', 'View roles'),
  ('system:role:create', 'Create roles', 'system', 'Create roles'),
  ('system:role:update', 'Update roles', 'system', 'Update roles'),
  ('system:role:delete', 'Delete roles', 'system', 'Delete roles'),
  ('system:menu:list', 'List menus', 'system', 'View menus'),
  ('system:menu:create', 'Create menus', 'system', 'Create menus'),
  ('system:menu:update', 'Update menus', 'system', 'Update menus'),
  ('system:menu:delete', 'Delete menus', 'system', 'Delete menus'),
  ('system:module:list', 'List modules', 'system', 'View registered modules'),
  ('system:dictionary:list', 'List dictionaries', 'system', 'View dictionaries'),
  ('system:dictionary:create', 'Create dictionaries', 'system', 'Create dictionaries'),
  ('system:dictionary:update', 'Update dictionaries', 'system', 'Update dictionaries'),
  ('system:dictionary:delete', 'Delete dictionaries', 'system', 'Delete dictionaries'),
  ('system:setting:list', 'List settings', 'system', 'View system settings'),
  ('system:setting:create', 'Create settings', 'system', 'Create system settings'),
  ('system:setting:update', 'Update settings', 'system', 'Update system settings'),
  ('system:setting:delete', 'Delete settings', 'system', 'Delete system settings'),
  ('system:audit:list', 'List audit logs', 'system', 'View audit logs')
ON CONFLICT (code) DO UPDATE SET
  name = EXCLUDED.name,
  module = EXCLUDED.module,
  description = EXCLUDED.description;

INSERT INTO gov2_roles (name, code, description)
SELECT seed.name, seed.code, seed.description
FROM (
  VALUES
    ('Administrator', 'admin', 'Full system access'),
    ('Operator', 'operator', 'Daily system operations')
) AS seed(name, code, description)
WHERE NOT EXISTS (
  SELECT 1
  FROM gov2_roles r
  WHERE lower(r.code) = lower(seed.code)
    AND r.deleted_at IS NULL
);

INSERT INTO gov2_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM gov2_roles r
JOIN gov2_permissions p ON p.code = '*'
WHERE r.code = 'admin'
ON CONFLICT DO NOTHING;

INSERT INTO gov2_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM gov2_roles r
JOIN gov2_permissions p ON p.code IN (
  'dashboard:view',
  'system:user:list',
  'system:role:list',
  'system:menu:list',
  'system:module:list',
  'system:dictionary:list',
  'system:setting:list',
  'system:audit:list'
)
WHERE r.code = 'operator'
ON CONFLICT DO NOTHING;

INSERT INTO gov2_menus (title, name, path, icon, component, permission_code, sort)
VALUES
  ('Dashboard', 'dashboard', '/dashboard', 'layout-dashboard', 'DashboardView', 'dashboard:view', 10),
  ('System', 'system', '/system', 'settings', 'Layout', NULL, 20)
ON CONFLICT (lower(name)) WHERE deleted_at IS NULL DO UPDATE SET
  title = EXCLUDED.title,
  path = EXCLUDED.path,
  icon = EXCLUDED.icon,
  component = EXCLUDED.component,
  permission_code = EXCLUDED.permission_code,
  sort = EXCLUDED.sort;

INSERT INTO gov2_menus (parent_id, title, name, path, icon, component, permission_code, sort)
SELECT parent.id, child.title, child.name, child.path, child.icon, child.component, child.permission_code, child.sort
FROM gov2_menus parent
JOIN (
  VALUES
    ('Users', 'system-users', '/system/users', 'users', 'UsersView', 'system:user:list', 21),
    ('Roles', 'system-roles', '/system/roles', 'shield', 'RolesView', 'system:role:list', 22),
    ('Menus', 'system-menus', '/system/menus', 'menu', 'MenusView', 'system:menu:list', 23),
    ('Modules', 'system-modules', '/system/modules', 'boxes', 'ModulesView', 'system:module:list', 24),
    ('Dictionaries', 'system-dictionaries', '/system/dictionaries', 'book', 'DictionariesView', 'system:dictionary:list', 25),
    ('Settings', 'system-settings', '/system/settings', 'sliders-horizontal', 'SettingsView', 'system:setting:list', 26),
    ('Audit Logs', 'system-audit', '/system/audit', 'history', 'AuditLogsView', 'system:audit:list', 27)
) AS child(title, name, path, icon, component, permission_code, sort) ON parent.name = 'system'
ON CONFLICT (lower(name)) WHERE deleted_at IS NULL DO UPDATE SET
  parent_id = EXCLUDED.parent_id,
  title = EXCLUDED.title,
  path = EXCLUDED.path,
  icon = EXCLUDED.icon,
  component = EXCLUDED.component,
  permission_code = EXCLUDED.permission_code,
  sort = EXCLUDED.sort;

INSERT INTO gov2_dictionaries (code, name, description)
VALUES ('user_status', 'User Status', 'Built-in user status dictionary')
ON CONFLICT (lower(code)) WHERE deleted_at IS NULL DO UPDATE SET
  name = EXCLUDED.name,
  description = EXCLUDED.description;

INSERT INTO gov2_dictionary_items (dictionary_id, label, value, sort)
SELECT d.id, item.label, item.value, item.sort
FROM gov2_dictionaries d
JOIN (
  VALUES
    ('Active', 'active', 1),
    ('Disabled', 'disabled', 2)
) AS item(label, value, sort) ON d.code = 'user_status'
ON CONFLICT (dictionary_id, value) DO UPDATE SET
  label = EXCLUDED.label,
  sort = EXCLUDED.sort,
  status = 'active';

INSERT INTO gov2_settings ("key", value_json, description)
VALUES ('site.title', '"GOV2"'::jsonb, 'Displayed application title')
ON CONFLICT (lower("key")) DO NOTHING;
