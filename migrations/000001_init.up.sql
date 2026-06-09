CREATE TABLE gov2_users (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  username TEXT NOT NULL,
  nickname TEXT NOT NULL DEFAULT '',
  email TEXT,
  phone TEXT,
  avatar TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  last_login_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  created_by BIGINT,
  updated_by BIGINT,
  version BIGINT NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX gov2_users_username_lower_uq ON gov2_users (lower(username)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX gov2_users_email_lower_uq ON gov2_users (lower(email)) WHERE email IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX gov2_users_phone_uq ON gov2_users (phone) WHERE phone IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE gov2_roles (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  code TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  created_by BIGINT,
  updated_by BIGINT,
  version BIGINT NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX gov2_roles_code_lower_uq ON gov2_roles (lower(code)) WHERE deleted_at IS NULL;

CREATE TABLE gov2_permissions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  module TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE gov2_user_roles (
  user_id BIGINT NOT NULL REFERENCES gov2_users(id) ON DELETE CASCADE,
  role_id BIGINT NOT NULL REFERENCES gov2_roles(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role_id)
);

CREATE INDEX gov2_user_roles_role_id_idx ON gov2_user_roles (role_id);

CREATE TABLE gov2_role_permissions (
  role_id BIGINT NOT NULL REFERENCES gov2_roles(id) ON DELETE CASCADE,
  permission_id BIGINT NOT NULL REFERENCES gov2_permissions(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX gov2_role_permissions_permission_id_idx ON gov2_role_permissions (permission_id);

CREATE TABLE gov2_menus (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  parent_id BIGINT REFERENCES gov2_menus(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  icon TEXT NOT NULL DEFAULT '',
  component TEXT NOT NULL DEFAULT '',
  permission_code TEXT REFERENCES gov2_permissions(code) ON UPDATE CASCADE,
  sort INTEGER NOT NULL DEFAULT 0,
  hidden BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX gov2_menus_parent_sort_idx ON gov2_menus (parent_id, sort);
CREATE UNIQUE INDEX gov2_menus_name_lower_uq ON gov2_menus (lower(name)) WHERE deleted_at IS NULL;

CREATE TABLE gov2_dictionaries (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX gov2_dictionaries_code_lower_uq ON gov2_dictionaries (lower(code)) WHERE deleted_at IS NULL;

CREATE TABLE gov2_dictionary_items (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dictionary_id BIGINT NOT NULL REFERENCES gov2_dictionaries(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  value TEXT NOT NULL,
  sort INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (dictionary_id, value)
);

CREATE INDEX gov2_dictionary_items_dictionary_sort_idx ON gov2_dictionary_items (dictionary_id, sort);

CREATE TABLE gov2_audit_logs (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT,
  actor TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  resource TEXT NOT NULL,
  resource_id TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  detail_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX gov2_audit_logs_created_at_idx ON gov2_audit_logs (created_at DESC);
CREATE INDEX gov2_audit_logs_actor_created_idx ON gov2_audit_logs (actor_id, created_at DESC);
CREATE INDEX gov2_audit_logs_action_created_idx ON gov2_audit_logs (action, created_at DESC);
CREATE INDEX gov2_audit_logs_resource_created_idx ON gov2_audit_logs (resource, created_at DESC);

CREATE TABLE gov2_settings (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  "key" TEXT NOT NULL,
  value_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX gov2_settings_key_lower_uq ON gov2_settings (lower("key"));

CREATE TABLE gov2_sessions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES gov2_users(id) ON DELETE CASCADE,
  refresh_token_hash TEXT NOT NULL UNIQUE,
  user_agent TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX gov2_sessions_user_id_idx ON gov2_sessions (user_id);
CREATE INDEX gov2_sessions_expires_at_idx ON gov2_sessions (expires_at);
