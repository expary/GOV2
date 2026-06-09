import { apiPath } from "./endpoints.js";
import { openApiSchemas } from "./types.js";

const tokenKey = "gov2_token";

export class ApiError extends Error {
  constructor({ status, code, message, requestID, data }) {
    super(message || "Request failed");
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.requestID = requestID;
    this.data = data;
  }

  get unauthorized() {
    return this.status === 401;
  }
}

export async function api(path, options = {}) {
  const hasBody = options.body !== undefined;
  const headers = {
    ...(hasBody ? { "Content-Type": "application/json" } : {}),
    ...(options.headers || {}),
  };
  const token = localStorage.getItem(tokenKey);
  if (options.auth !== false && token) {
    headers.Authorization = `Bearer ${token}`;
  }

  const response = await fetch(path, {
    method: options.method || "GET",
    headers,
    body: hasBody ? JSON.stringify(options.body) : undefined,
    signal: options.signal,
  });

  if (response.ok && options.responseType === "text") {
    return response.text();
  }

  const payload = await response.json().catch(() => ({
    message: "Invalid server response",
  }));

  if (!response.ok) {
    assertErrorEnvelope(payload);
    const error = new ApiError({
      status: response.status,
      code: payload.code,
      message: payload.message,
      requestID: payload.request_id || response.headers.get("X-Request-ID"),
      data: payload.data,
    });
    if (options.auth !== false && error.unauthorized) {
      localStorage.removeItem(tokenKey);
      window.dispatchEvent(new CustomEvent("gov2:auth-expired", { detail: error }));
    }
    throw error;
  }

  assertSuccessEnvelope(payload);
  return payload.data;
}

export async function apiRequest(endpoint, options = {}) {
  const { params, query, auth, ...requestOptions } = options;
  validateRequest(endpoint, params || {}, query || {}, requestOptions);
  const path = `${apiPath(endpoint, params)}${buildQuery(query || {})}`;
  const data = await api(path, {
    method: endpoint.method,
    auth: auth ?? (endpoint.public ? false : undefined),
    responseType: endpoint.responseType,
    ...requestOptions,
  });
  validateResponse(endpoint, data);
  return data;
}

export function buildQuery(params) {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (Array.isArray(value)) {
      value.forEach((item) => {
        if (item !== undefined && item !== null && item !== "") {
          query.append(key, item);
        }
      });
    } else if (value !== undefined && value !== null && value !== "") {
      query.set(key, value);
    }
  });
  const text = query.toString();
  return text ? `?${text}` : "";
}

function validateRequest(endpoint, params, query, options) {
  if (!endpoint?.method || !endpoint?.path) {
    throw new Error("Invalid API endpoint metadata");
  }
  assertKnownSchema("body", endpoint.bodySchema, endpoint);
  assertKnownSchema("response", endpoint.responseSchema, endpoint);

  assertKnownKeys("path parameter", params, endpoint.pathParams || [], endpoint);
  assertKnownKeys("query parameter", query, endpoint.queryParams || [], endpoint);
  assertParameterTypes("path parameter", params, endpoint.pathParamTypes || {}, endpoint, false);
  assertParameterTypes("query parameter", query, endpoint.queryParamTypes || {}, endpoint, true);

  if (endpoint.bodyRequired && options.body === undefined) {
    throw new Error(`Missing request body for ${endpoint.method} ${endpoint.path}`);
  }
  if (!endpoint.body && options.body !== undefined) {
    throw new Error(`Unexpected request body for ${endpoint.method} ${endpoint.path}`);
  }
  if (endpoint.body && options.body !== undefined) {
    assertBodyContract(options.body, endpoint);
  }
}

function assertSuccessEnvelope(payload) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error("Invalid API response envelope");
  }
  if (!Object.prototype.hasOwnProperty.call(payload, "data")) {
    throw new Error('Missing API response envelope field "data"');
  }
  assertEnvelopeFieldTypes(payload);
}

function assertErrorEnvelope(payload) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error("Invalid API error response envelope");
  }
  assertEnvelopeFieldTypes(payload);
}

function assertEnvelopeFieldTypes(payload) {
  if (payload.code !== undefined && !matchesContractType(payload.code, "number")) {
    throw new Error('Expected number API response envelope field "code"');
  }
  if (payload.message !== undefined && !matchesContractType(payload.message, "string")) {
    throw new Error('Expected string API response envelope field "message"');
  }
  if (payload.request_id !== undefined && !matchesContractType(payload.request_id, "string")) {
    throw new Error('Expected string API response envelope field "request_id"');
  }
}

function assertKnownKeys(kind, values, allowed, endpoint) {
  if (!values || Object.keys(values).length === 0) return;
  const allowedSet = new Set(allowed);
  Object.keys(values).forEach((key) => {
    if (!allowedSet.has(key)) {
      throw new Error(`Unknown API ${kind} "${key}" for ${endpoint.method} ${endpoint.path}`);
    }
  });
}

function assertParameterTypes(kind, values, types, endpoint, allowArrays) {
  Object.entries(values || {}).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") return;
    const expectedType = types[key];
    if (Array.isArray(value)) {
      if (!allowArrays) {
        throw new Error(`Expected scalar API ${kind} "${key}" for ${endpoint.method} ${endpoint.path}`);
      }
      if (!expectedType) return;
      value.forEach((item, index) => {
        if (!matchesContractType(item, expectedType)) {
          throw new Error(`Expected ${expectedType} API ${kind} "${key}" item ${index} for ${endpoint.method} ${endpoint.path}`);
        }
      });
      return;
    }
    if (!expectedType) return;
    if (!matchesContractType(value, expectedType)) {
      throw new Error(`Expected ${expectedType} API ${kind} "${key}" for ${endpoint.method} ${endpoint.path}`);
    }
  });
}

function assertBodyContract(body, endpoint) {
  const allowedFields = endpoint.bodyFields || [];
  const requiredFields = endpoint.bodyRequiredFields || [];
  const schema = endpoint.bodySchema ? openApiSchemas[endpoint.bodySchema] : undefined;
  if (allowedFields.length === 0 && requiredFields.length === 0) return;
  if (!body || typeof body !== "object" || Array.isArray(body)) {
    throw new Error(`Expected object request body for ${endpoint.method} ${endpoint.path}`);
  }

  assertKnownKeys("body field", body, allowedFields, endpoint);
  requiredFields.forEach((field) => {
    if (body[field] === undefined || body[field] === null) {
      throw new Error(`Missing API body field "${field}" for ${endpoint.method} ${endpoint.path}`);
    }
  });
  Object.entries(schema?.fieldTypes || {}).forEach(([field, expectedType]) => {
    const value = body[field];
    if (value === undefined || value === null) return;
    if (!matchesContractType(value, expectedType)) {
      throw new Error(`Expected ${expectedType} API body field "${field}" for ${endpoint.method} ${endpoint.path}`);
    }
  });
  Object.entries(schema?.enumValues || {}).forEach(([field, allowedValues]) => {
    const value = body[field];
    if (value === undefined || value === null) return;
    if (!allowedValues.includes(value)) {
      throw new Error(`Unexpected API body field "${field}" value ${JSON.stringify(value)} for ${endpoint.method} ${endpoint.path}; expected one of ${formatEnumValues(allowedValues)}`);
    }
  });
  Object.entries(schema?.arrayScalarItemTypes || {}).forEach(([field, expectedType]) => {
    const value = body[field];
    if (value === undefined || value === null) return;
    if (!Array.isArray(value)) return;
    value.forEach((item, index) => {
      if (!matchesContractType(item, expectedType)) {
        throw new Error(`Expected ${expectedType} API body field "${field}" item ${index} for ${endpoint.method} ${endpoint.path}`);
      }
    });
  });
  Object.entries(schema?.arrayObjectItemFields || {}).forEach(([field, itemFields]) => {
    const value = body[field];
    if (value === undefined || value === null) return;
    if (!Array.isArray(value)) return;
    value.forEach((item, index) => {
      assertBodyArrayObjectItemContract(item, endpoint, field, index, itemFields, schema);
    });
  });
}

function assertBodyArrayObjectItemContract(item, endpoint, field, index, itemFields, schema) {
  if (!item || typeof item !== "object" || Array.isArray(item)) {
    throw new Error(`Expected object API body field "${field}" item ${index} for ${endpoint.method} ${endpoint.path}`);
  }
  assertKnownKeys(`body field "${field}" item ${index} field`, item, itemFields, endpoint);
  (schema.arrayObjectItemRequiredFields?.[field] || []).forEach((itemField) => {
    if (item[itemField] === undefined || item[itemField] === null) {
      throw new Error(`Missing API body field "${field}" item ${index} field "${itemField}" for ${endpoint.method} ${endpoint.path}`);
    }
  });
  Object.entries(schema.arrayObjectItemFieldTypes?.[field] || {}).forEach(([itemField, expectedType]) => {
    const value = item[itemField];
    if (value === undefined || value === null) return;
    if (!matchesContractType(value, expectedType)) {
      throw new Error(`Expected ${expectedType} API body field "${field}" item ${index} field "${itemField}" for ${endpoint.method} ${endpoint.path}`);
    }
  });
}

function validateResponse(endpoint, data) {
  if (!endpoint.responseSchema) return;
  validateResponseSchema(endpoint.responseSchema, data, endpoint, "");
}

function validateResponseSchema(schemaName, data, endpoint, location) {
  const schema = openApiSchemas[schemaName];
  if (schema.type === "array") {
    if (!Array.isArray(data)) {
      throw new Error(`Expected array response data for ${endpoint.method} ${endpoint.path} response schema "${schemaName}"`);
    }
    const itemSchemaName = schema.itemType;
    const itemSchema = itemSchemaName ? openApiSchemas[itemSchemaName] : undefined;
    if (itemSchema?.type === "object") {
      data.forEach((item, index) => {
        assertObjectResponseContract(item, endpoint, schemaName, itemSchemaName, `${location} item ${index}`.trim());
      });
    }
    return;
  }
  if (schema.type === "object") {
    assertObjectResponseContract(data, endpoint, schemaName, schemaName, location);
  }
}

function assertObjectResponseContract(data, endpoint, responseSchemaName, objectSchemaName, location) {
  if (!data || typeof data !== "object" || Array.isArray(data)) {
    throw new Error(`Expected object response data for ${endpoint.method} ${endpoint.path} response schema "${responseSchemaName}"${formatSchemaLocation(location, objectSchemaName)}`);
  }
  const schema = openApiSchemas[objectSchemaName];
  for (const field of schema.required || []) {
    if (!Object.prototype.hasOwnProperty.call(data, field)) {
      throw new Error(`Missing API response field "${field}" for ${endpoint.method} ${endpoint.path} response schema "${responseSchemaName}"${formatSchemaLocation(location, objectSchemaName)}`);
    }
  }
  Object.entries(schema.fieldTypes || {}).forEach(([field, expectedType]) => {
    const value = data[field];
    if (value === undefined || value === null) return;
    if (!matchesContractType(value, expectedType)) {
      throw new Error(`Expected ${expectedType} API response field "${field}" for ${endpoint.method} ${endpoint.path} response schema "${responseSchemaName}"${formatSchemaLocation(location, objectSchemaName)}`);
    }
  });
  Object.entries(schema.enumValues || {}).forEach(([field, allowedValues]) => {
    const value = data[field];
    if (value === undefined || value === null) return;
    if (!allowedValues.includes(value)) {
      throw new Error(`Unexpected API response field "${field}" value ${JSON.stringify(value)} for ${endpoint.method} ${endpoint.path} response schema "${responseSchemaName}"${formatSchemaLocation(location, objectSchemaName)}; expected one of ${formatEnumValues(allowedValues)}`);
    }
  });
  Object.entries(schema.objectFieldTypes || {}).forEach(([field, fieldSchemaName]) => {
    const value = data[field];
    if (value === undefined || value === null) return;
    const fieldSchema = openApiSchemas[fieldSchemaName];
    if (fieldSchema?.type === "object") {
      assertObjectResponseContract(value, endpoint, responseSchemaName, fieldSchemaName, formatFieldLocation(location, field));
    }
  });
  Object.entries(schema.arrayItemTypes || {}).forEach(([field, itemSchemaName]) => {
    const value = data[field];
    if (value === undefined || value === null) return;
    if (!Array.isArray(value)) {
      throw new Error(`Expected array response field "${field}" for ${endpoint.method} ${endpoint.path} response schema "${responseSchemaName}"${formatSchemaLocation(location, objectSchemaName)}`);
    }
    const itemSchema = openApiSchemas[itemSchemaName];
    if (itemSchema?.type === "object") {
      value.forEach((item, index) => {
        assertObjectResponseContract(item, endpoint, responseSchemaName, itemSchemaName, formatNestedLocation(location, field, index));
      });
    }
  });
  Object.entries(schema.arrayScalarItemTypes || {}).forEach(([field, expectedType]) => {
    const value = data[field];
    if (value === undefined || value === null) return;
    if (!Array.isArray(value)) {
      throw new Error(`Expected array response field "${field}" for ${endpoint.method} ${endpoint.path} response schema "${responseSchemaName}"${formatSchemaLocation(location, objectSchemaName)}`);
    }
    value.forEach((item, index) => {
      if (!matchesContractType(item, expectedType)) {
        throw new Error(`Expected ${expectedType} API response field "${field}" item ${index} for ${endpoint.method} ${endpoint.path} response schema "${responseSchemaName}"${formatSchemaLocation(location, objectSchemaName)}`);
      }
    });
  });
}

function formatSchemaLocation(location, schemaName) {
  return location ? ` ${location} schema "${schemaName}"` : "";
}

function formatNestedLocation(location, field, index) {
  return `${location ? `${location} ` : ""}field "${field}" item ${index}`;
}

function formatFieldLocation(location, field) {
  return `${location ? `${location} ` : ""}field "${field}"`;
}

function matchesContractType(value, expectedType) {
  if (expectedType === "array") return Array.isArray(value);
  if (expectedType === "object") return !!value && typeof value === "object" && !Array.isArray(value);
  if (expectedType === "number") return typeof value === "number" && Number.isFinite(value);
  return typeof value === expectedType;
}

function formatEnumValues(values) {
  return values.map((value) => JSON.stringify(value)).join(", ");
}

function assertKnownSchema(kind, name, endpoint) {
  if (!name) return;
  if (!openApiSchemas[name]) {
    throw new Error(`Unknown API ${kind} schema "${name}" for ${endpoint.method} ${endpoint.path}`);
  }
}

export function validationErrorsByField(error) {
  const fields = error?.data?.fields;
  if (!Array.isArray(fields)) {
    return {};
  }
  return fields.reduce((out, item) => {
    if (item?.field && item?.message && !out[item.field]) {
      out[item.field] = item.message;
    }
    return out;
  }, {});
}
