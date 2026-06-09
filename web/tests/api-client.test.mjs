import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { api, apiRequest, buildQuery, validationErrorsByField } from "../src/api/client.js";
import { apiEndpoints } from "../src/api/endpoints.js";

test("generated endpoint metadata and request JSDoc include operation summaries", () => {
  for (const [name, endpoint] of Object.entries(apiEndpoints)) {
    assert.equal(typeof endpoint.summary, "string", `${name} summary type`);
    assert.notEqual(endpoint.summary.trim(), "", `${name} summary`);
  }
  assert.equal(apiEndpoints.getHealth.summary, "Health check");

  const requestSource = readFileSync(new URL("../src/api/requests.js", import.meta.url), "utf8");
  assert.match(requestSource, /\/\*\*\n \* Health check\n \* @param \{object\} \[options\]\n[\s\S]*?export function getHealth/);
});

test("buildQuery omits empty values and repeats arrays", () => {
  assert.equal(
    buildQuery({
      page: 1,
      keyword: "alice smith",
      empty: "",
      missing: null,
      tags: ["active", "", "admin"],
    }),
    "?page=1&keyword=alice+smith&tags=active&tags=admin",
  );
});

test("apiRequest validates endpoint metadata, params, query, and body fields", async () => {
  const endpoint = Object.freeze({
    method: "PUT",
    path: "/api/v1/system/users/{id}",
    pathParams: Object.freeze(["id"]),
    pathParamTypes: Object.freeze({ id: "number" }),
    queryParams: Object.freeze(["dry_run"]),
    queryParamTypes: Object.freeze({ dry_run: "boolean" }),
    body: true,
    bodyRequired: true,
    bodyFields: Object.freeze(["username"]),
    bodyRequiredFields: Object.freeze(["username"]),
  });

  await assert.rejects(() => apiRequest(endpoint, { params: { extra: 1 }, body: { username: "alice" } }), {
    message: 'Unknown API path parameter "extra" for PUT /api/v1/system/users/{id}',
  });
  await assert.rejects(() => apiRequest(endpoint, { params: { id: 1 }, query: { unknown: true }, body: { username: "alice" } }), {
    message: 'Unknown API query parameter "unknown" for PUT /api/v1/system/users/{id}',
  });
  await assert.rejects(() => apiRequest(endpoint, { params: { id: "one" }, body: { username: "alice" } }), {
    message: 'Expected number API path parameter "id" for PUT /api/v1/system/users/{id}',
  });
  await assert.rejects(() => apiRequest(endpoint, { params: { id: [1, 2] }, body: { username: "alice" } }), {
    message: 'Expected scalar API path parameter "id" for PUT /api/v1/system/users/{id}',
  });
  await assert.rejects(() => apiRequest(endpoint, { params: { id: 1 }, query: { dry_run: "yes" }, body: { username: "alice" } }), {
    message: 'Expected boolean API query parameter "dry_run" for PUT /api/v1/system/users/{id}',
  });
  await assert.rejects(() => apiRequest(endpoint, { params: { id: 1 }, query: { dry_run: [true, "yes"] }, body: { username: "alice" } }), {
    message: 'Expected boolean API query parameter "dry_run" item 1 for PUT /api/v1/system/users/{id}',
  });
  await assert.rejects(() => apiRequest(endpoint, { params: { id: 1 } }), {
    message: "Missing request body for PUT /api/v1/system/users/{id}",
  });
  await assert.rejects(() => apiRequest(endpoint, { params: { id: 1 }, body: { nickname: "Alice" } }), {
    message: 'Unknown API body field "nickname" for PUT /api/v1/system/users/{id}',
  });

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          ...endpoint,
          bodySchema: "UpdateUserRequest",
          bodyFields: Object.freeze(["username", "role_ids", "status"]),
          bodyRequiredFields: Object.freeze(["username"]),
        }),
        {
          params: { id: 1 },
          body: { username: 123 },
        },
      ),
    {
      message: 'Expected string API body field "username" for PUT /api/v1/system/users/{id}',
    },
  );

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "PATCH",
          path: "/api/v1/system/users/{id}/status",
          pathParams: Object.freeze(["id"]),
          body: true,
          bodyRequired: true,
          bodySchema: "UserStatusRequest",
          bodyFields: Object.freeze(["status"]),
          bodyRequiredFields: Object.freeze(["status"]),
        }),
        {
          params: { id: 1 },
          body: { status: "archived" },
        },
      ),
    {
      message: 'Unexpected API body field "status" value "archived" for PATCH /api/v1/system/users/{id}/status; expected one of "active", "disabled"',
    },
  );

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          ...endpoint,
          bodySchema: "UpdateUserRequest",
          bodyFields: Object.freeze(["username", "role_ids", "status"]),
          bodyRequiredFields: Object.freeze(["username"]),
        }),
        {
          params: { id: 1 },
          body: { username: "alice", role_ids: [1, "operator"] },
        },
      ),
    {
      message: 'Expected number API body field "role_ids" item 1 for PUT /api/v1/system/users/{id}',
    },
  );

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "POST",
          path: "/api/v1/system/dictionaries",
          body: true,
          bodyRequired: true,
          bodySchema: "DictionaryRequest",
          bodyFields: Object.freeze(["code", "name", "items"]),
          bodyRequiredFields: Object.freeze(["code", "name"]),
        }),
        {
          body: { code: "status", name: "Status", items: [{ value: "active" }] },
        },
      ),
    {
      message: 'Missing API body field "items" item 0 field "label" for POST /api/v1/system/dictionaries',
    },
  );

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "POST",
          path: "/api/v1/system/dictionaries",
          body: true,
          bodyRequired: true,
          bodySchema: "DictionaryRequest",
          bodyFields: Object.freeze(["code", "name", "items"]),
          bodyRequiredFields: Object.freeze(["code", "name"]),
        }),
        {
          body: { code: "status", name: "Status", items: [{ label: "Active", value: "active", sort: "first" }] },
        },
      ),
    {
      message: 'Expected number API body field "items" item 0 field "sort" for POST /api/v1/system/dictionaries',
    },
  );

  await assert.rejects(
    () =>
      apiRequest(Object.freeze({ method: "GET", path: "/api/v1/system/users", body: false }), {
        body: { unexpected: true },
      }),
    {
      message: "Unexpected request body for GET /api/v1/system/users",
    },
  );
});

test("apiRequest validates generated response schema metadata", async () => {
  globalThis.localStorage = memoryStorage("token-1");
  globalThis.fetch = async () =>
    jsonResponse(200, {
      code: 200,
      data: { user_count: 1, role_count: 2, audit_log_count: 3, active_user_count: 1 },
      request_id: "req-schema",
    });

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "GET",
          path: "/api/v1/dashboard/summary",
          responseSchema: "DashboardSummary",
        }),
      ),
    {
      message: 'Missing API response field "menu_count" for GET /api/v1/dashboard/summary response schema "DashboardSummary"',
    },
  );

  globalThis.fetch = async () =>
    jsonResponse(200, {
      code: 200,
      data: { user_count: "1", role_count: 2, menu_count: 3, audit_log_count: 4, active_user_count: 1 },
      request_id: "req-field-type",
    });

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "GET",
          path: "/api/v1/dashboard/summary",
          responseSchema: "DashboardSummary",
        }),
      ),
    {
      message: 'Expected number API response field "user_count" for GET /api/v1/dashboard/summary response schema "DashboardSummary"',
    },
  );

  globalThis.fetch = async () =>
    jsonResponse(200, {
      code: 200,
      data: { items: [] },
      request_id: "req-array",
    });

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "GET",
          path: "/api/v1/system/roles",
          responseSchema: "RoleList",
        }),
      ),
    {
      message: 'Expected array response data for GET /api/v1/system/roles response schema "RoleList"',
    },
  );

  globalThis.fetch = async () =>
    jsonResponse(200, {
      code: 200,
      data: [
        {
          id: 1,
          name: "Operators",
          description: "Operators",
          permissions: [],
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      request_id: "req-array-item",
    });

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "GET",
          path: "/api/v1/system/roles",
          responseSchema: "RoleList",
        }),
      ),
    {
      message: 'Missing API response field "code" for GET /api/v1/system/roles response schema "RoleList" item 0 schema "Role"',
    },
  );

  globalThis.fetch = async () =>
    jsonResponse(200, {
      code: 200,
      data: {
        items: [
          {
            id: 1,
            username: "alice",
            nickname: "Alice",
            email: "",
            phone: "",
            avatar: "",
            role_ids: [],
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
          },
        ],
        total: 1,
        page: 1,
        page_size: 20,
      },
      request_id: "req-page-item",
    });

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "GET",
          path: "/api/v1/system/users",
          responseSchema: "UserPage",
        }),
      ),
    {
      message: 'Missing API response field "status" for GET /api/v1/system/users response schema "UserPage" field "items" item 0 schema "PublicUser"',
    },
  );

  globalThis.fetch = async () =>
    jsonResponse(200, {
      code: 200,
      data: {
        token: "token-1",
        expires_at: 123,
        user: {
          id: 1,
          username: "alice",
          nickname: "Alice",
          email: "",
          phone: "",
          avatar: "",
          role_ids: [],
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      },
      request_id: "req-object-field",
    });

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "POST",
          path: "/api/v1/auth/login",
          responseSchema: "LoginResponse",
        }),
      ),
    {
      message: 'Missing API response field "status" for POST /api/v1/auth/login response schema "LoginResponse" field "user" schema "PublicUser"',
    },
  );

  globalThis.fetch = async () =>
    jsonResponse(200, {
      code: 200,
      data: {
        token: "token-1",
        expires_at: 123,
        user: {
          id: 1,
          username: "alice",
          nickname: "Alice",
          email: "",
          phone: "",
          avatar: "",
          role_ids: [],
          status: "archived",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      },
      request_id: "req-enum-field",
    });

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "POST",
          path: "/api/v1/auth/login",
          responseSchema: "LoginResponse",
        }),
      ),
    {
      message: 'Unexpected API response field "status" value "archived" for POST /api/v1/auth/login response schema "LoginResponse" field "user" schema "PublicUser"; expected one of "active", "disabled"',
    },
  );

  globalThis.fetch = async () =>
    jsonResponse(200, {
      code: 200,
      data: {
        user: {
          id: 1,
          username: "alice",
          nickname: "Alice",
          email: "",
          phone: "",
          avatar: "",
          role_ids: [],
          status: "active",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
        roles: [1, "operator"],
        permissions: ["dashboard:view"],
        menus: [],
      },
      request_id: "req-array-scalar-item",
    });

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "GET",
          path: "/api/v1/auth/profile",
          responseSchema: "AuthProfile",
        }),
      ),
    {
      message: 'Expected number API response field "roles" item 1 for GET /api/v1/auth/profile response schema "AuthProfile"',
    },
  );
});

test("apiRequest rejects unknown generated schema metadata", async () => {
  globalThis.localStorage = memoryStorage("token-1");
  globalThis.fetch = async () => jsonResponse(200, { code: 200, data: {}, request_id: "req-unused" });

  await assert.rejects(
    () =>
      apiRequest(
        Object.freeze({
          method: "GET",
          path: "/api/v1/test",
          responseSchema: "MissingSchema",
        }),
      ),
    {
      message: 'Unknown API response schema "MissingSchema" for GET /api/v1/test',
    },
  );
});

test("api serializes JSON bodies and unwraps response data", async () => {
  const calls = [];
  globalThis.localStorage = memoryStorage("token-1");
  globalThis.fetch = async (path, options) => {
    calls.push({ path, options });
    return jsonResponse(200, { code: 200, data: { ok: true }, request_id: "req-1" });
  };

  const result = await api("/api/v1/test", { method: "POST", body: { name: "alice" } });

  assert.deepEqual(result, { ok: true });
  assert.equal(calls[0].path, "/api/v1/test");
  assert.equal(calls[0].options.headers["Content-Type"], "application/json");
  assert.equal(calls[0].options.headers.Authorization, "Bearer token-1");
  assert.equal(calls[0].options.body, JSON.stringify({ name: "alice" }));
});

test("api validates successful JSON response envelopes before unwrapping", async () => {
  globalThis.localStorage = memoryStorage("token-1");

  globalThis.fetch = async () => jsonResponse(200, null);
  await assert.rejects(() => api("/api/v1/test"), {
    message: "Invalid API response envelope",
  });

  globalThis.fetch = async () => jsonResponse(200, { code: 200, request_id: "req-missing-data" });
  await assert.rejects(() => api("/api/v1/test"), {
    message: 'Missing API response envelope field "data"',
  });

  globalThis.fetch = async () => jsonResponse(200, { code: "200", data: { ok: true }, request_id: "req-bad-code" });
  await assert.rejects(() => api("/api/v1/test"), {
    message: 'Expected number API response envelope field "code"',
  });

  globalThis.fetch = async () => jsonResponse(200, { code: 200, message: 123, data: { ok: true }, request_id: "req-bad-message" });
  await assert.rejects(() => api("/api/v1/test"), {
    message: 'Expected string API response envelope field "message"',
  });

  globalThis.fetch = async () => jsonResponse(200, { code: 200, data: { ok: true }, request_id: 123 });
  await assert.rejects(() => api("/api/v1/test"), {
    message: 'Expected string API response envelope field "request_id"',
  });
});

test("apiRequest honors public endpoints and forwards abort signals", async () => {
  const calls = [];
  const signal = AbortSignal.timeout(1000);
  globalThis.localStorage = memoryStorage("token-1");
  globalThis.fetch = async (path, options) => {
    calls.push({ path, options });
    return jsonResponse(200, { code: 200, data: { ok: true }, request_id: "req-public" });
  };

  const result = await apiRequest(
    Object.freeze({
      method: "GET",
      path: "/api/v1/app/config",
      public: true,
      queryParams: Object.freeze(["environment"]),
    }),
    {
      query: { environment: "dev" },
      signal,
    },
  );

  assert.deepEqual(result, { ok: true });
  assert.equal(calls[0].path, "/api/v1/app/config?environment=dev");
  assert.equal(calls[0].options.headers.Authorization, undefined);
  assert.equal(calls[0].options.signal, signal);
});

test("api clears auth and dispatches expiration event on authenticated 401", async () => {
  const storage = memoryStorage("token-1");
  const events = [];
  globalThis.localStorage = storage;
  globalThis.window = {
    dispatchEvent(event) {
      events.push(event);
    },
  };
  globalThis.CustomEvent = class CustomEvent {
    constructor(type, init = {}) {
      this.type = type;
      this.detail = init.detail;
    }
  };
  globalThis.fetch = async () =>
    jsonResponse(401, {
      code: 401,
      message: "invalid bearer token",
      request_id: "req-401",
    });

  await assert.rejects(() => api("/api/v1/protected"), (error) => {
    assert.equal(error.status, 401);
    assert.equal(error.requestID, "req-401");
    return true;
  });
  assert.equal(storage.getItem("gov2_token"), null);
  assert.equal(events.length, 1);
  assert.equal(events[0].type, "gov2:auth-expired");
});

test("api validates JSON error response envelopes before creating ApiError", async () => {
  globalThis.localStorage = memoryStorage("token-1");

  globalThis.fetch = async () => jsonResponse(500, null);
  await assert.rejects(() => api("/api/v1/error"), {
    message: "Invalid API error response envelope",
  });

  globalThis.fetch = async () => jsonResponse(500, { code: "500", message: "server error", request_id: "req-error-code" });
  await assert.rejects(() => api("/api/v1/error"), {
    message: 'Expected number API response envelope field "code"',
  });

  globalThis.fetch = async () => jsonResponse(500, { code: 500, message: 123, request_id: "req-error-message" });
  await assert.rejects(() => api("/api/v1/error"), {
    message: 'Expected string API response envelope field "message"',
  });

  globalThis.fetch = async () => jsonResponse(500, { code: 500, message: "server error", request_id: 123 });
  await assert.rejects(() => api("/api/v1/error"), {
    message: 'Expected string API response envelope field "request_id"',
  });

  globalThis.fetch = async () => invalidJsonResponse(502, "req-header-fallback");
  await assert.rejects(() => api("/api/v1/error"), (error) => {
    assert.equal(error.status, 502);
    assert.equal(error.message, "Invalid server response");
    assert.equal(error.requestID, "req-header-fallback");
    return true;
  });
});

test("validationErrorsByField returns the first message per field", () => {
  assert.deepEqual(
    validationErrorsByField({
      data: {
        fields: [
          { field: "username", message: "required" },
          { field: "username", message: "duplicate" },
          { field: "email", message: "invalid" },
        ],
      },
    }),
    { username: "required", email: "invalid" },
  );
});

function jsonResponse(status, payload) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: {
      get(name) {
        return name === "X-Request-ID" ? payload.request_id : "";
      },
    },
    async json() {
      return payload;
    },
  };
}

function invalidJsonResponse(status, requestID) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: {
      get(name) {
        return name === "X-Request-ID" ? requestID : "";
      },
    },
    async json() {
      throw new Error("invalid json");
    },
  };
}

function memoryStorage(initialToken = "") {
  const values = new Map();
  if (initialToken) {
    values.set("gov2_token", initialToken);
  }
  return {
    getItem(key) {
      return values.has(key) ? values.get(key) : null;
    },
    setItem(key, value) {
      values.set(key, value);
    },
    removeItem(key) {
      values.delete(key);
    },
  };
}
