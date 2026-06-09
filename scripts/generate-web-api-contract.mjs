import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const openAPIPath = path.join(rootDir, "api", "openapi.yaml");
const domainPermissionsPath = path.join(rootDir, "internal", "domain", "permissions.go");
const endpointOutputPath = path.join(rootDir, "web", "src", "api", "endpoints.js");
const requestOutputPath = path.join(rootDir, "web", "src", "api", "requests.js");
const typeOutputPath = path.join(rootDir, "web", "src", "api", "types.js");
const permissionOutputPath = path.join(rootDir, "web", "src", "permissions.js");
const checkOnly = process.argv.includes("--check");

const spec = fs.readFileSync(openAPIPath, "utf8");
const permissionSource = fs.readFileSync(domainPermissionsPath, "utf8");
const specLines = spec.split(/\r?\n/);
const componentSchemas = parseComponentSchemas(specLines);
const componentParameters = parseComponentParameters(specLines);
const componentResponses = parseComponentResponses(specLines);
const operations = parseOpenAPI(spec, componentSchemas);
const operationErrors = validateOpenAPIOperations(operations, componentSchemas, componentParameters, componentResponses);
if (operationErrors.length > 0) {
  console.error(operationErrors.join("\n"));
  process.exit(1);
}
const permissions = parseGoPermissions(permissionSource);
const outputs = [
  {
    path: endpointOutputPath,
    content: renderEndpoints(operations),
  },
  {
    path: requestOutputPath,
    content: renderRequests(operations),
  },
  {
    path: typeOutputPath,
    content: renderTypes(Array.from(componentSchemas.values())),
  },
  {
    path: permissionOutputPath,
    content: renderPermissions(permissions),
  },
];

if (checkOnly) {
  const stale = outputs.filter((output) => {
    const current = fs.existsSync(output.path) ? fs.readFileSync(output.path, "utf8") : "";
    return current !== output.content;
  });
  if (stale.length > 0) {
    const files = stale.map((output) => path.relative(rootDir, output.path)).join(", ");
    console.error(`${files} out of date. Run: npm run api:generate`);
    process.exit(1);
  }
  console.log("web API endpoint, schema type, and permission contracts are up to date");
} else {
  outputs.forEach((output) => {
    fs.writeFileSync(output.path, output.content);
  });
  console.log(`generated ${outputs.map((output) => path.relative(rootDir, output.path)).join(", ")}`);
}

function parseOpenAPI(source, componentSchemas) {
  const lines = source.split(/\r?\n/);
  let basePath = "";
  let currentPath = "";
  let currentMethod = "";
  let currentRequestMediaType = "";
  let currentResponseStatus = "";
  let inPaths = false;
  let section = "";
  let directParameter = null;
  const componentParameters = parseComponentParameters(lines);
  const operations = [];

  const flushDirectParameter = () => {
    if (!directParameter || operations.length === 0) return;
    addOperationParameter(operations[operations.length - 1], directParameter);
    directParameter = null;
  };

  for (const line of lines) {
    const trimmed = line.trim();
    if (!basePath && trimmed.startsWith("- url:")) {
      basePath = unquote(trimmed.slice("- url:".length).trim());
      continue;
    }
    if (trimmed === "paths:") {
      inPaths = true;
      continue;
    }
    if (trimmed === "components:") {
      flushDirectParameter();
      inPaths = false;
      currentPath = "";
      currentMethod = "";
      currentRequestMediaType = "";
      currentResponseStatus = "";
      section = "";
      continue;
    }
    if (!inPaths) continue;

    if (line.startsWith("  /") && !line.startsWith("    ") && trimmed.endsWith(":")) {
      flushDirectParameter();
      currentPath = trimmed.slice(0, -1);
      currentMethod = "";
      currentRequestMediaType = "";
      section = "";
      continue;
    }
    if (!currentPath) continue;

    if (line.startsWith("    ") && !line.startsWith("      ") && trimmed.endsWith(":")) {
      flushDirectParameter();
      const method = trimmed.slice(0, -1);
      if (!isHTTPMethod(method)) continue;
      currentMethod = method.toUpperCase();
      currentRequestMediaType = "";
      currentResponseStatus = "";
      operations.push({
        name: operationName(method, currentPath),
        method: currentMethod,
        path: `${basePath}${currentPath}`,
        summary: "",
        public: false,
        permission: "",
        parameterRefs: [],
        pathParams: pathParamNames(currentPath),
        pathParamTypes: [],
        pathParamSchemaTypes: [],
        declaredPathParams: [],
        optionalPathParams: [],
        queryParams: [],
        queryParamTypes: [],
        queryParamSchemaTypes: [],
        requestBody: false,
        bodyRequired: false,
        bodySchema: "",
        bodySchemaRef: "",
        bodyFields: [],
        bodyRequiredFields: [],
        requestMediaTypes: [],
        responseSchema: "",
        responseSchemaRef: "",
        responseRefs: [],
        responseRefsByStatus: new Map(),
        responseStatuses: [],
        successStatuses: [],
        successMediaTypes: [],
      });
      section = "";
      continue;
    }
    if (!currentMethod || !line.startsWith("      ")) {
      continue;
    }

    const operation = operations[operations.length - 1];
    const indent = leadingSpaces(line);
    if (indent === 6) {
      flushDirectParameter();
      section = "";
      currentRequestMediaType = "";
      currentResponseStatus = "";
    }
    if (trimmed === "security: []") {
      operation.public = true;
    } else if (indent === 6 && trimmed.startsWith("summary:")) {
      operation.summary = unquote(trimmed.slice("summary:".length).trim());
    } else if (trimmed.startsWith("x-permission:")) {
      operation.permission = unquote(trimmed.slice("x-permission:".length).trim());
    } else if (indent === 6 && trimmed === "parameters:") {
      section = "parameters";
    } else if (indent === 6 && trimmed === "requestBody:") {
      section = "requestBody";
      operation.requestBody = true;
    } else if (indent === 6 && trimmed === "responses:") {
      section = "responses";
    } else if (section === "parameters") {
      if (indent === 8 && trimmed.startsWith("- $ref:")) {
        const refName = refTail(trimmed.slice("- $ref:".length).trim());
        pushUnique(operation.parameterRefs, refName);
        addOperationParameter(operation, componentParameters.get(refName));
      } else if (indent === 8 && trimmed.startsWith("- name:")) {
        flushDirectParameter();
        directParameter = {
          name: unquote(trimmed.slice("- name:".length).trim()),
          in: "",
          required: false,
          type: "",
          schemaType: "",
        };
      } else if (indent === 10 && directParameter && trimmed.startsWith("in:")) {
        directParameter.in = unquote(trimmed.slice("in:".length).trim());
      } else if (indent === 10 && directParameter && trimmed.startsWith("required:")) {
        directParameter.required = trimmed.slice("required:".length).trim() === "true";
      } else if (directParameter && trimmed.startsWith("type:")) {
        setParameterSchemaType(directParameter, unquote(trimmed.slice("type:".length).trim()));
      }
    } else if (section === "requestBody" && indent === 8 && trimmed.startsWith("required:")) {
      operation.bodyRequired = trimmed.slice("required:".length).trim() === "true";
    } else if (section === "requestBody" && indent === 10 && isMediaType(trimmed)) {
      currentRequestMediaType = unquote(trimmed.slice(0, -1));
      pushUnique(operation.requestMediaTypes, currentRequestMediaType);
    } else if (section === "requestBody" && currentRequestMediaType === "application/json" && trimmed.startsWith("$ref:")) {
      const schemaName = refTail(trimmed.slice("$ref:".length).trim());
      operation.bodySchemaRef = schemaName;
      const schema = componentSchemas.get(schemaName);
      if (schema) {
        operation.bodySchema = schemaName;
        operation.bodyFields = schema.fields;
        operation.bodyRequiredFields = schema.required;
      }
    } else if (section === "responses") {
      if (indent === 8 && trimmed.endsWith(":")) {
        currentResponseStatus = unquote(trimmed.slice(0, -1));
        pushUnique(operation.responseStatuses, currentResponseStatus);
        if (isSuccessResponse(currentResponseStatus)) {
          pushUnique(operation.successStatuses, currentResponseStatus);
        }
      } else if (currentResponseStatus && indent === 10 && trimmed.startsWith("$ref:")) {
        const refName = refTail(trimmed.slice("$ref:".length).trim());
        pushUnique(operation.responseRefs, refName);
        pushOperationResponseRef(operation, currentResponseStatus, refName);
      } else if (isSuccessResponse(currentResponseStatus) && isMediaType(trimmed)) {
        pushUnique(operation.successMediaTypes, unquote(trimmed.slice(0, -1)));
      } else if (isSuccessResponse(currentResponseStatus) && trimmed.startsWith("$ref:")) {
        const schemaName = refTail(trimmed.slice("$ref:".length).trim());
        operation.responseSchemaRef = schemaName;
        if (componentSchemas.has(schemaName)) {
          operation.responseSchema = schemaName;
        }
      }
    }
  }
  flushDirectParameter();

  return operations;
}

function validateOpenAPIOperations(operations, componentSchemas, componentParameters, componentResponses) {
  const errors = [];
  const operationNames = new Map();
  for (const schema of componentSchemas.values()) {
    collectRequiredPropertyErrors(schema, `components.schemas.${schema.name}`, errors);
    collectArrayItemErrors(schema, `components.schemas.${schema.name}`, errors);
    collectUnknownTypeErrors(schema, `components.schemas.${schema.name}`, errors);
    for (const refName of schemaRefs(schema)) {
      if (!componentSchemas.has(refName)) {
        errors.push(`components.schemas.${schema.name}: unknown schema "${refName}"`);
      }
    }
  }
  for (const [responseName, response] of componentResponses.entries()) {
    for (const refName of response.schemaRefs) {
      if (!componentSchemas.has(refName)) {
        errors.push(`components.responses.${responseName}: unknown schema "${refName}"`);
      }
    }
  }
  for (const [responseName, schemaName] of standardErrorResponseSchemas()) {
    const response = componentResponses.get(responseName);
    if (!response) {
      errors.push(`components.responses.${responseName}: missing standard error response component`);
      continue;
    }
    if (!response.mediaTypes.includes("application/json")) {
      errors.push(`components.responses.${responseName}: must document application/json content`);
    }
    if (!response.schemaRefs.includes(schemaName)) {
      errors.push(`components.responses.${responseName}: must reference schema "${schemaName}"`);
    }
  }
  for (const operation of operations) {
    const label = `${operation.method} ${operation.path}`;
    const existingOperation = operationNames.get(operation.name);
    if (existingOperation) {
      errors.push(`${label}: generated frontend operation name "${operation.name}" conflicts with ${existingOperation}`);
    } else {
      operationNames.set(operation.name, label);
    }
    if (operation.successStatuses.length === 0) {
      errors.push(`${label}: missing 2xx success response`);
    }
    if (!operation.summary) {
      errors.push(`${label}: missing non-empty operation summary`);
    }
    if (operation.public && operation.permission) {
      errors.push(`${label}: public operation must not declare x-permission`);
    }
    if (!operation.public && !responseRefsForStatus(operation, "401").includes("UnauthorizedError")) {
      errors.push(`${label}: protected operation must document a 401 UnauthorizedError response`);
    }
    if (operation.permission && !responseRefsForStatus(operation, "403").includes("ForbiddenError")) {
      errors.push(`${label}: permission-protected operation must document a 403 ForbiddenError response`);
    }
    for (const mediaType of operation.successMediaTypes) {
      if (!supportedSuccessMediaType(mediaType)) {
        errors.push(`${label}: unsupported success response media type "${mediaType}"`);
      }
    }
    if (operation.successMediaTypes.includes("text/plain") && !plainTextSuccessResponse(operation)) {
      errors.push(`${label}: text/plain success responses must be the only success response media type`);
    }
    if (operation.requestBody) {
      if (!operation.bodyRequired) {
        errors.push(`${label}: request body operation must document required: true`);
      }
      if (!operation.requestMediaTypes.includes("application/json")) {
        errors.push(`${label}: request body operation must document application/json content`);
      }
      for (const status of ["400", "413", "415"]) {
        if (!operation.responseStatuses.includes(status)) {
          errors.push(`${label}: request body operation must document a ${status} response`);
        }
      }
      const badRequestRefs = responseRefsForStatus(operation, "400");
      if (!badRequestRefs.includes("BadRequestError") && !badRequestRefs.includes("ValidationError")) {
        errors.push(`${label}: request body operation must document a 400 BadRequestError or ValidationError response`);
      }
      if (!responseRefsForStatus(operation, "413").includes("PayloadTooLargeError")) {
        errors.push(`${label}: request body operation must document a 413 PayloadTooLargeError response`);
      }
      if (!responseRefsForStatus(operation, "415").includes("UnsupportedMediaTypeError")) {
        errors.push(`${label}: request body operation must document a 415 UnsupportedMediaTypeError response`);
      }
    } else {
      for (const status of ["413", "415"]) {
        if (operation.responseStatuses.includes(status)) {
          errors.push(`${label}: non-request body operation must not document a ${status} JSON request-body response`);
        }
      }
    }
    for (const refName of operation.parameterRefs) {
      if (!componentParameters.has(refName)) {
        errors.push(`${label}: unknown parameter "${refName}"`);
      }
    }
    for (const refName of operation.responseRefs) {
      if (!componentResponses.has(refName)) {
        errors.push(`${label}: unknown response "${refName}"`);
      }
    }
    for (const param of operation.pathParams) {
      if (!operation.declaredPathParams.includes(param)) {
        errors.push(`${label}: missing OpenAPI path parameter declaration for "${param}"`);
      }
    }
    for (const param of operation.declaredPathParams) {
      if (!operation.pathParams.includes(param)) {
        errors.push(`${label}: path parameter "${param}" is declared but not present in path`);
      }
    }
    for (const param of operation.optionalPathParams) {
      errors.push(`${label}: path parameter "${param}" must be required`);
    }
    for (const [param, schemaType] of operation.pathParamSchemaTypes) {
      if (!schemaType) {
        errors.push(`${label}: path parameter "${param}" must declare a schema type`);
      } else if (!runtimeParameterType(schemaType)) {
        errors.push(`${label}: path parameter "${param}" uses unsupported schema type "${schemaType}"`);
      }
    }
    for (const [param, schemaType] of operation.queryParamSchemaTypes) {
      if (!schemaType) {
        errors.push(`${label}: query parameter "${param}" must declare a schema type`);
      } else if (!runtimeParameterType(schemaType)) {
        errors.push(`${label}: query parameter "${param}" uses unsupported schema type "${schemaType}"`);
      }
    }
    if (operation.responseSchemaRef && !componentSchemas.has(operation.responseSchemaRef)) {
      errors.push(`${label}: unknown success response schema "${operation.responseSchemaRef}"`);
    }
    if (operation.bodySchemaRef && !componentSchemas.has(operation.bodySchemaRef)) {
      errors.push(`${label}: unknown request body schema "${operation.bodySchemaRef}"`);
    }
    if (operation.requestBody && !operation.bodySchemaRef) {
      errors.push(`${label}: missing request body schema for generated frontend request contract`);
    }
    if (requiresSuccessResponseSchema(operation) && !operation.responseSchema) {
      errors.push(`${label}: missing success response schema for generated frontend return type`);
    }
  }
  return errors;
}

function collectRequiredPropertyErrors(schema, label, errors) {
  if (!schema || typeof schema !== "object") return;
  const properties = schema.properties || {};
  for (const field of schema.required || []) {
    if (!Object.prototype.hasOwnProperty.call(properties, field)) {
      errors.push(`${label}: required field "${field}" is not declared in properties`);
    }
  }
  if (schema.items) {
    collectRequiredPropertyErrors(schema.items, `${label}.items`, errors);
  }
  for (const [field, property] of Object.entries(properties)) {
    if (property?.items) {
      collectRequiredPropertyErrors(property.items, `${label}.${field}.items`, errors);
    }
  }
}

function collectArrayItemErrors(schema, label, errors) {
  if (!schema || typeof schema !== "object") return;
  if (schema.type === "array" && !schema.items) {
    errors.push(`${label}: array schema must declare items`);
  }
  for (const [field, property] of Object.entries(schema.properties || {})) {
    const propertyLabel = `${label}.${field}`;
    if (property.type === "array" && !property.items) {
      errors.push(`${propertyLabel}: array schema property must declare items`);
    }
    if (property.items) {
      collectArrayItemErrors(property.items, `${propertyLabel}.items`, errors);
    }
  }
  if (schema.items) {
    collectArrayItemErrors(schema.items, `${label}.items`, errors);
  }
}

function collectUnknownTypeErrors(schema, label, errors) {
  if (!schema || typeof schema !== "object") return;
  for (const [field, property] of Object.entries(schema.properties || {})) {
    const propertyLabel = `${label}.${field}`;
    if (jsDocType(property) === "unknown" && !allowsUnknownSchemaValue(property)) {
      errors.push(`${propertyLabel}: schema property would generate unknown type without an explicit arbitrary JSON description`);
    }
    if (property.items) {
      collectUnknownTypeErrors(property.items, `${propertyLabel}.items`, errors);
      if (jsDocArrayItemType(property.items) === "unknown" && !allowsUnknownSchemaValue(property.items)) {
        errors.push(`${propertyLabel}.items: array item would generate unknown type without an explicit arbitrary JSON description`);
      }
    }
  }
  if (schema.items) {
    collectUnknownTypeErrors(schema.items, `${label}.items`, errors);
    if (jsDocArrayItemType(schema.items) === "unknown" && !allowsUnknownSchemaValue(schema.items)) {
      errors.push(`${label}.items: array item would generate unknown type without an explicit arbitrary JSON description`);
    }
  }
}

function allowsUnknownSchemaValue(value) {
  return /any valid json value/i.test(value?.description || "");
}

function schemaRefs(schema) {
  const refs = [];
  collectSchemaRefs(schema, refs);
  return refs;
}

function collectSchemaRefs(value, refs) {
  if (!value || typeof value !== "object") return;
  if (value.ref) {
    pushUnique(refs, value.ref);
  }
  if (value.items) {
    collectSchemaRefs(value.items, refs);
  }
  if (value.properties) {
    for (const property of Object.values(value.properties)) {
      collectSchemaRefs(property, refs);
    }
  }
}

function requiresSuccessResponseSchema(operation) {
  if (operation.successStatuses.length === 0) return false;
  if (operation.successStatuses.every((status) => status === "204")) return false;
  if (plainTextSuccessResponse(operation)) return false;
  return true;
}

function plainTextSuccessResponse(operation) {
  return operation.successMediaTypes.length === 1 && operation.successMediaTypes[0] === "text/plain";
}

function supportedSuccessMediaType(mediaType) {
  return mediaType === "application/json" || mediaType === "text/plain";
}

function standardErrorResponseSchemas() {
  return new Map([
    ["BadRequestError", "ErrorEnvelope"],
    ["UnauthorizedError", "ErrorEnvelope"],
    ["ForbiddenError", "ErrorEnvelope"],
    ["NotFoundError", "ErrorEnvelope"],
    ["ConflictError", "ErrorEnvelope"],
    ["PayloadTooLargeError", "ErrorEnvelope"],
    ["UnsupportedMediaTypeError", "ErrorEnvelope"],
    ["ValidationError", "ValidationErrorEnvelope"],
  ]);
}

function renderEndpoints(operations) {
  const entries = operations
    .map((operation) => {
      const fields = [
        `method: ${JSON.stringify(operation.method)}`,
        `path: ${JSON.stringify(operation.path)}`,
        `summary: ${JSON.stringify(operation.summary)}`,
      ];
      if (operation.public) fields.push("public: true");
      if (operation.permission) fields.push(`permission: ${JSON.stringify(operation.permission)}`);
      if (plainTextSuccessResponse(operation)) fields.push(`responseType: "text"`);
      if (operation.responseSchema) fields.push(`responseSchema: ${JSON.stringify(operation.responseSchema)}`);
      if (operation.pathParams.length > 0) {
        fields.push(`pathParams: Object.freeze(${JSON.stringify(operation.pathParams)})`);
      }
      if (operation.pathParamTypes.length > 0) {
        fields.push(`pathParamTypes: Object.freeze(${JSON.stringify(Object.fromEntries(operation.pathParamTypes))})`);
      }
      if (operation.queryParams.length > 0) {
        fields.push(`queryParams: Object.freeze(${JSON.stringify(operation.queryParams)})`);
      }
      if (operation.queryParamTypes.length > 0) {
        fields.push(`queryParamTypes: Object.freeze(${JSON.stringify(Object.fromEntries(operation.queryParamTypes))})`);
      }
      if (operation.requestBody) fields.push("body: true");
      if (operation.bodyRequired) fields.push("bodyRequired: true");
      if (operation.bodySchema) fields.push(`bodySchema: ${JSON.stringify(operation.bodySchema)}`);
      if (operation.bodyFields.length > 0) {
        fields.push(`bodyFields: Object.freeze(${JSON.stringify(operation.bodyFields)})`);
      }
      if (operation.bodyRequiredFields.length > 0) {
        fields.push(`bodyRequiredFields: Object.freeze(${JSON.stringify(operation.bodyRequiredFields)})`);
      }
      return `  ${operation.name}: Object.freeze({ ${fields.join(", ")} }),`;
    })
    .join("\n");

  return `// Code generated by scripts/generate-web-api-contract.mjs. DO NOT EDIT.\n\nexport const apiEndpoints = Object.freeze({\n${entries}\n});\n\nexport function apiPath(endpoint, params = {}) {\n  return endpoint.path.replace(/\\{([^}]+)\\}/g, (_, key) => {\n    const value = params[key];\n    if (value === undefined || value === null || value === "") {\n      throw new Error(\`Missing API path parameter "\${key}"\`);\n    }\n    return encodeURIComponent(String(value));\n  });\n}\n`;
}

function renderRequests(operations) {
  const functions = operations.map(renderRequestFunction).join("\n\n");
  return `// Code generated by scripts/generate-web-api-contract.mjs. DO NOT EDIT.\n\nimport { apiRequest } from "@/api/client";\nimport { apiEndpoints } from "@/api/endpoints";\n\n${functions}\n`;
}

function renderRequestFunction(operation) {
  const lines = ["/**", ` * ${operation.summary}`, " * @param {object} [options]"];
  if (operation.pathParams.length > 0) {
    lines.push(` * @param {${parameterObjectType(operation.pathParams, operation.pathParamTypes, "string|number", false)}} options.params`);
  }
  if (operation.queryParams.length > 0) {
    lines.push(` * @param {${parameterObjectType(operation.queryParams, operation.queryParamTypes, "string|number|boolean", true)}} [options.query]`);
  }
  if (operation.bodySchema) {
    const bodyField = operation.bodyRequired ? "options.body" : "[options.body]";
    lines.push(` * @param {import("./types").${operation.bodySchema}} ${bodyField}`);
  } else if (operation.requestBody) {
    const bodyField = operation.bodyRequired ? "options.body" : "[options.body]";
    lines.push(` * @param {unknown} ${bodyField}`);
  }
  lines.push(" * @param {boolean} [options.auth]");
  lines.push(" * @param {AbortSignal} [options.signal]");
  lines.push(` * @returns {Promise<${responseType(operation)}>}`);
  lines.push(" */");
  lines.push(`export function ${operation.name}(options = {}) {`);
  lines.push(`  return apiRequest(apiEndpoints.${operation.name}, options);`);
  lines.push("}");
  return lines.join("\n");
}

function parseGoPermissions(source) {
  const permissions = [];
  for (const match of source.matchAll(/^\s*(Permission[A-Za-z0-9]+)\s*=\s*"([^"]+)"/gm)) {
    permissions.push({
      name: permissionExportName(match[1]),
      code: match[2],
    });
  }
  if (permissions.length === 0) {
    throw new Error("No backend permissions found in internal/domain/permissions.go");
  }
  return permissions;
}

function renderPermissions(permissions) {
  const entries = permissions
    .map((permission) => `  ${permission.name}: ${JSON.stringify(permission.code)},`)
    .join("\n");

  return `// Code generated by scripts/generate-web-api-contract.mjs. DO NOT EDIT.\n\nexport const permissions = Object.freeze({\n${entries}\n});\n\nexport const knownPermissions = Object.freeze(Object.values(permissions));\n`;
}

function renderTypes(schemas) {
  const typedefs = schemas.map(renderTypeDefinition).join("\n\n");
  const schemaEntries = schemas
    .map((schema) => {
      const fields = [
        `type: ${JSON.stringify(schemaKind(schema))}`,
        `fields: Object.freeze(${JSON.stringify(schema.fields)})`,
        `required: Object.freeze(${JSON.stringify(schema.required)})`,
      ];
      const fieldTypes = objectScalarFieldTypes(schema);
      if (fieldTypes.length > 0) {
        fields.push(`fieldTypes: Object.freeze(${JSON.stringify(Object.fromEntries(fieldTypes))})`);
      }
      const enumValues = objectEnumValues(schema);
      if (enumValues.length > 0) {
        fields.push(`enumValues: Object.freeze(${JSON.stringify(Object.fromEntries(enumValues))})`);
      }
      if (schemaKind(schema) === "array") {
        fields.push(`itemType: ${JSON.stringify(jsDocArrayItemType(schema.items || {}))}`);
      }
      const arrayItemTypes = objectArrayItemTypes(schema);
      if (arrayItemTypes.length > 0) {
        fields.push(`arrayItemTypes: Object.freeze(${JSON.stringify(Object.fromEntries(arrayItemTypes))})`);
      }
      const arrayScalarItemTypes = objectArrayScalarItemTypes(schema);
      if (arrayScalarItemTypes.length > 0) {
        fields.push(`arrayScalarItemTypes: Object.freeze(${JSON.stringify(Object.fromEntries(arrayScalarItemTypes))})`);
      }
      const arrayObjectItemFields = objectArrayObjectItemFields(schema);
      if (arrayObjectItemFields.length > 0) {
        fields.push(`arrayObjectItemFields: Object.freeze(${JSON.stringify(Object.fromEntries(arrayObjectItemFields))})`);
      }
      const arrayObjectItemRequiredFields = objectArrayObjectItemRequiredFields(schema);
      if (arrayObjectItemRequiredFields.length > 0) {
        fields.push(`arrayObjectItemRequiredFields: Object.freeze(${JSON.stringify(Object.fromEntries(arrayObjectItemRequiredFields))})`);
      }
      const arrayObjectItemFieldTypes = objectArrayObjectItemFieldTypes(schema);
      if (arrayObjectItemFieldTypes.length > 0) {
        fields.push(`arrayObjectItemFieldTypes: Object.freeze(${JSON.stringify(Object.fromEntries(arrayObjectItemFieldTypes))})`);
      }
      const objectFieldTypes = objectReferenceFields(schema);
      if (objectFieldTypes.length > 0) {
        fields.push(`objectFieldTypes: Object.freeze(${JSON.stringify(Object.fromEntries(objectFieldTypes))})`);
      }
      return `  ${schema.name}: Object.freeze({ ${fields.join(", ")} }),`;
    })
    .join("\n");

  return `// Code generated by scripts/generate-web-api-contract.mjs. DO NOT EDIT.\n\n${typedefs}\n\nexport const openApiSchemas = Object.freeze({\n${schemaEntries}\n});\n`;
}

function renderTypeDefinition(schema) {
  if (schemaKind(schema) === "array") {
    return ["/**", ` * @typedef {Array<${jsDocArrayItemType(schema.items || {})}>} ${schema.name}`, " */"].join("\n");
  }
  if (schema.ref) {
    return ["/**", ` * @typedef {${schema.ref}} ${schema.name}`, " */"].join("\n");
  }
  const lines = ["/**", ` * @typedef {object} ${schema.name}`];
  for (const field of schema.fields) {
    const property = schema.properties[field] || {};
    const type = jsDocType(property);
    const name = schema.required.includes(field) ? field : `[${field}]`;
    lines.push(` * @property {${type}} ${name}`);
  }
  lines.push(" */");
  return lines.join("\n");
}

function parseComponentSchemas(lines) {
  const schemas = new Map();
  let inComponents = false;
  let inSchemas = false;
  let currentName = "";
  let schema = null;
  let section = "";
  let currentProperty = null;
  let propertySection = "";
  let itemSection = "";
  let currentItemProperty = null;
  let schemaItemSection = "";
  let currentSchemaItemProperty = null;

  const flushSchema = () => {
    if (currentName && schema) {
      schemas.set(currentName, {
        name: currentName,
        type: schema.type,
        ref: schema.ref,
        items: schema.items,
        fields: schema.fields,
        required: schema.required,
        properties: schema.properties,
      });
    }
    currentName = "";
    schema = null;
    section = "";
    currentProperty = null;
    propertySection = "";
    itemSection = "";
    currentItemProperty = null;
    schemaItemSection = "";
    currentSchemaItemProperty = null;
  };

  for (const line of lines) {
    const trimmed = line.trim();
    const indent = leadingSpaces(line);
    if (trimmed === "components:") {
      inComponents = true;
      continue;
    }
    if (!inComponents) continue;

    if (indent === 2 && trimmed.endsWith(":")) {
      flushSchema();
      inSchemas = trimmed === "schemas:";
      continue;
    }
    if (!inSchemas) continue;

    if (indent === 4 && trimmed.endsWith(":")) {
      flushSchema();
      currentName = trimmed.slice(0, -1);
      schema = { type: "", ref: "", items: null, fields: [], required: [], properties: {} };
      continue;
    }
    if (!schema) continue;

    if (indent === 6 && trimmed.startsWith("type:")) {
      schema.type = unquote(trimmed.slice("type:".length).trim());
      continue;
    }
    if (indent === 6 && trimmed.startsWith("$ref:")) {
      schema.ref = refTail(trimmed.slice("$ref:".length).trim());
      continue;
    }
    if (indent === 6 && trimmed === "items:") {
      section = "schemaItems";
      schema.items = schema.items || { properties: {}, required: [] };
      currentProperty = null;
      continue;
    }
    if (indent === 6 && trimmed === "required:") {
      section = "required";
      currentProperty = null;
      continue;
    }
    if (indent === 6 && trimmed === "properties:") {
      section = "properties";
      currentProperty = null;
      continue;
    }
    if (indent <= 6 && trimmed && !trimmed.startsWith("- ")) {
      section = "";
      currentProperty = null;
    }

    if (section === "required" && indent === 8 && trimmed.startsWith("- ")) {
      pushUnique(schema.required, unquote(trimmed.slice(2).trim()));
    } else if (section === "schemaItems") {
      parseItemSchemaLine(schema.items, trimmed, indent, {
        get section() {
          return schemaItemSection;
        },
        set section(value) {
          schemaItemSection = value;
        },
        get property() {
          return currentSchemaItemProperty;
        },
        set property(value) {
          currentSchemaItemProperty = value;
        },
      }, 8);
    } else if (section === "properties" && indent === 8 && trimmed.endsWith(":")) {
      currentProperty = trimmed.slice(0, -1);
      schema.properties[currentProperty] = schema.properties[currentProperty] || {};
      pushUnique(schema.fields, currentProperty);
      propertySection = "";
      itemSection = "";
      currentItemProperty = null;
    } else if (section === "properties" && currentProperty) {
      const property = schema.properties[currentProperty];
      if (indent === 10 && trimmed.startsWith("type:")) {
        property.type = unquote(trimmed.slice("type:".length).trim());
      } else if (indent === 10 && trimmed.startsWith("format:")) {
        property.format = unquote(trimmed.slice("format:".length).trim());
      } else if (indent === 10 && trimmed.startsWith("$ref:")) {
        property.ref = refTail(trimmed.slice("$ref:".length).trim());
      } else if (indent === 10 && trimmed === "enum:") {
        propertySection = "enum";
      } else if (propertySection === "enum" && indent === 12 && trimmed.startsWith("- ")) {
        property.enum = property.enum || [];
        pushUnique(property.enum, unquote(trimmed.slice(2).trim()));
      } else if (indent === 10 && trimmed === "items:") {
        propertySection = "items";
        property.items = property.items || { properties: {}, required: [] };
      } else if (propertySection === "items") {
        parseItemSchemaLine(property.items, trimmed, indent, {
          get section() {
            return itemSection;
          },
          set section(value) {
            itemSection = value;
          },
          get property() {
            return currentItemProperty;
          },
          set property(value) {
            currentItemProperty = value;
          },
        }, 12);
      } else if (indent === 10 && trimmed.startsWith("description:")) {
        property.description = unquote(trimmed.slice("description:".length).trim());
      }
    }
  }
  flushSchema();

  return schemas;
}

function parseItemSchemaLine(item, trimmed, indent, state, baseIndent) {
  if (indent === baseIndent && trimmed.startsWith("type:")) {
    item.type = unquote(trimmed.slice("type:".length).trim());
  } else if (indent === baseIndent && trimmed.startsWith("format:")) {
    item.format = unquote(trimmed.slice("format:".length).trim());
  } else if (indent === baseIndent && trimmed.startsWith("$ref:")) {
    item.ref = refTail(trimmed.slice("$ref:".length).trim());
  } else if (indent === baseIndent && trimmed === "required:") {
    state.section = "required";
    state.property = null;
  } else if (state.section === "required" && indent === baseIndent + 2 && trimmed.startsWith("- ")) {
    pushUnique(item.required, unquote(trimmed.slice(2).trim()));
  } else if (indent === baseIndent && trimmed === "properties:") {
    state.section = "properties";
    state.property = null;
  } else if (state.section === "properties" && indent === baseIndent + 2 && trimmed.endsWith(":")) {
    state.property = trimmed.slice(0, -1);
    item.properties[state.property] = item.properties[state.property] || {};
  } else if (state.section === "properties" && state.property && indent === baseIndent + 4) {
    const property = item.properties[state.property];
    if (trimmed.startsWith("type:")) {
      property.type = unquote(trimmed.slice("type:".length).trim());
    } else if (trimmed.startsWith("format:")) {
      property.format = unquote(trimmed.slice("format:".length).trim());
    } else if (trimmed.startsWith("$ref:")) {
      property.ref = refTail(trimmed.slice("$ref:".length).trim());
    } else if (trimmed.startsWith("description:")) {
      property.description = unquote(trimmed.slice("description:".length).trim());
    }
  }
}

function schemaKind(schema) {
  if (schema.type) return schema.type;
  if (schema.items) return "array";
  if (schema.ref) return "ref";
  return "object";
}

function jsDocType(property) {
  if (property.ref) return property.ref;
  if (property.enum?.length > 0) {
    return property.enum.map((value) => JSON.stringify(value)).join("|");
  }
  if (property.type === "array") {
    return `Array<${jsDocArrayItemType(property.items || {})}>`;
  }
  if (property.type === "integer" || property.type === "number") return "number";
  if (property.type === "boolean") return "boolean";
  if (property.type === "object") return "Record<string, unknown>";
  if (property.type === "string") return "string";
  return "unknown";
}

function jsDocArrayItemType(item) {
  if (item.ref) return item.ref;
  if (item.type === "integer" || item.type === "number") return "number";
  if (item.type === "boolean") return "boolean";
  if (item.type === "string") return "string";
  if (item.type === "object" && item.properties) {
    const entries = Object.entries(item.properties).map(([name, property]) => {
      const optional = item.required?.includes(name) ? "" : "?";
      return `${name}${optional}: ${jsDocType(property)}`;
    });
    return `{ ${entries.join(", ")} }`;
  }
  return "unknown";
}

function objectScalarFieldTypes(schema) {
  return Object.entries(schema.properties || {})
    .map(([field, property]) => [field, runtimeFieldType(property)])
    .filter(([, type]) => type);
}

function runtimeFieldType(property) {
  if (property.ref) return "";
  if (property.type === "integer" || property.type === "number") return "number";
  if (property.type === "boolean") return "boolean";
  if (property.type === "string") return "string";
  if (property.type === "array") return "array";
  if (property.type === "object") return "object";
  return "";
}

function objectEnumValues(schema) {
  return Object.entries(schema.properties || {})
    .filter(([, property]) => property.enum?.length > 0)
    .map(([field, property]) => [field, property.enum]);
}

function objectArrayItemTypes(schema) {
  return Object.entries(schema.properties || {})
    .filter(([, property]) => property.type === "array" && property.items?.ref)
    .map(([field, property]) => [field, property.items.ref]);
}

function objectArrayScalarItemTypes(schema) {
  return Object.entries(schema.properties || {})
    .map(([field, property]) => [field, runtimeArrayItemType(property)])
    .filter(([, type]) => type);
}

function runtimeArrayItemType(property) {
  if (property.type !== "array" || !property.items || property.items.ref) return "";
  return runtimeFieldType(property.items);
}

function objectArrayObjectItemFields(schema) {
  return Object.entries(schema.properties || {})
    .filter(([, property]) => property.type === "array" && property.items?.type === "object")
    .map(([field, property]) => [field, Object.keys(property.items.properties || {})])
    .filter(([, fields]) => fields.length > 0);
}

function objectArrayObjectItemRequiredFields(schema) {
  return Object.entries(schema.properties || {})
    .filter(([, property]) => property.type === "array" && property.items?.type === "object")
    .map(([field, property]) => [field, property.items.required || []])
    .filter(([, fields]) => fields.length > 0);
}

function objectArrayObjectItemFieldTypes(schema) {
  return Object.entries(schema.properties || {})
    .filter(([, property]) => property.type === "array" && property.items?.type === "object")
    .map(([field, property]) => [
      field,
      Object.fromEntries(
        Object.entries(property.items.properties || {})
          .map(([itemField, itemProperty]) => [itemField, runtimeFieldType(itemProperty)])
          .filter(([, type]) => type),
      ),
    ])
    .filter(([, fields]) => Object.keys(fields).length > 0);
}

function objectReferenceFields(schema) {
  return Object.entries(schema.properties || {})
    .filter(([, property]) => property.ref)
    .map(([field, property]) => [field, property.ref]);
}

function parseComponentParameters(lines) {
  const parameters = new Map();
  let inComponents = false;
  let inParameters = false;
  let currentName = "";
  let parameter = null;

  const flushParameter = () => {
    if (currentName && parameter?.name && parameter?.in) {
      parameters.set(currentName, parameter);
    }
    currentName = "";
    parameter = null;
  };

  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed === "components:") {
      inComponents = true;
      continue;
    }
    if (!inComponents) continue;

    if (line.startsWith("  ") && !line.startsWith("    ") && trimmed.endsWith(":")) {
      flushParameter();
      inParameters = trimmed === "parameters:";
      continue;
    }
    if (!inParameters) continue;

    if (line.startsWith("    ") && !line.startsWith("      ") && trimmed.endsWith(":")) {
      flushParameter();
      currentName = trimmed.slice(0, -1);
      parameter = { name: "", in: "", required: false, type: "", schemaType: "" };
      continue;
    }
    if (!parameter || !line.startsWith("      ")) {
      continue;
    }

    if (trimmed.startsWith("name:")) {
      parameter.name = unquote(trimmed.slice("name:".length).trim());
    } else if (trimmed.startsWith("in:")) {
      parameter.in = unquote(trimmed.slice("in:".length).trim());
    } else if (trimmed.startsWith("required:")) {
      parameter.required = trimmed.slice("required:".length).trim() === "true";
    } else if (trimmed.startsWith("type:")) {
      setParameterSchemaType(parameter, unquote(trimmed.slice("type:".length).trim()));
    }
  }
  flushParameter();

  return parameters;
}

function parseComponentResponses(lines) {
  const responses = new Map();
  let inComponents = false;
  let inResponses = false;
  let currentName = "";

  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed === "components:") {
      inComponents = true;
      continue;
    }
    if (!inComponents) continue;

    if (line.startsWith("  ") && !line.startsWith("    ") && trimmed.endsWith(":")) {
      inResponses = trimmed === "responses:";
      currentName = "";
      continue;
    }
    if (!inResponses) continue;

    if (line.startsWith("    ") && !line.startsWith("      ") && trimmed.endsWith(":")) {
      currentName = trimmed.slice(0, -1);
      responses.set(currentName, { mediaTypes: [], schemaRefs: [] });
      continue;
    }
    if (currentName && isMediaType(trimmed)) {
      const response = responses.get(currentName);
      pushUnique(response.mediaTypes, unquote(trimmed.slice(0, -1)));
    }
    if (currentName && trimmed.startsWith("$ref:")) {
      const response = responses.get(currentName);
      pushUnique(response.schemaRefs, refTail(trimmed.slice("$ref:".length).trim()));
    }
  }

  return responses;
}

function addOperationParameter(operation, parameter) {
  if (!operation || !parameter?.name) return;
  if (parameter.in === "path") {
    pushUnique(operation.declaredPathParams, parameter.name);
    pushUniquePair(operation.pathParamSchemaTypes, parameter.name, parameter.schemaType);
    pushUniquePair(operation.pathParamTypes, parameter.name, parameter.type);
    if (!parameter.required) {
      pushUnique(operation.optionalPathParams, parameter.name);
    }
  } else if (parameter.in === "query") {
    pushUnique(operation.queryParams, parameter.name);
    pushUniquePair(operation.queryParamSchemaTypes, parameter.name, parameter.schemaType);
    pushUniquePair(operation.queryParamTypes, parameter.name, parameter.type);
  }
}

function pushOperationResponseRef(operation, status, refName) {
  const refs = operation.responseRefsByStatus.get(status) || [];
  pushUnique(refs, refName);
  operation.responseRefsByStatus.set(status, refs);
}

function responseRefsForStatus(operation, status) {
  return operation.responseRefsByStatus.get(status) || [];
}

function pathParamNames(apiPath) {
  return Array.from(apiPath.matchAll(/\{([^}]+)\}/g), (match) => match[1]);
}

function refTail(value) {
  return unquote(value).split("/").pop();
}

function pushUnique(items, value) {
  if (!items.includes(value)) items.push(value);
}

function pushUniquePair(items, key, value) {
  if (items.some(([existingKey]) => existingKey === key)) return;
  items.push([key, value]);
}

function setParameterSchemaType(parameter, schemaType) {
  parameter.schemaType = schemaType;
  parameter.type = runtimeParameterType(schemaType);
}

function runtimeParameterType(type) {
  if (type === "integer" || type === "number") return "number";
  if (type === "boolean") return "boolean";
  if (type === "string") return "string";
  return "";
}

function leadingSpaces(value) {
  return value.length - value.trimStart().length;
}

function operationName(method, apiPath) {
  const parts = apiPath
    .split("/")
    .filter(Boolean)
    .map((part) => part.replace(/[{}]/g, ""))
    .map(toPascalCase);
  return `${method.toLowerCase()}${parts.join("")}`;
}

function permissionExportName(name) {
  const withoutPrefix = name.replace(/^Permission/, "");
  return withoutPrefix.charAt(0).toLowerCase() + withoutPrefix.slice(1);
}

function objectType(fields, type, optional) {
  const entries = fields.map((field) => `${field}${optional ? "?" : ""}: ${type}`);
  return `{ ${entries.join(", ")} }`;
}

function parameterObjectType(fields, types, fallbackType, optional) {
  const typeMap = new Map(types);
  const entries = fields.map((field) => `${field}${optional ? "?" : ""}: ${typeMap.get(field) || fallbackType}`);
  return `{ ${entries.join(", ")} }`;
}

function responseType(operation) {
  if (plainTextSuccessResponse(operation)) return "string";
  return operation.responseSchema ? `import("./types").${operation.responseSchema}` : "unknown";
}

function toPascalCase(value) {
  return value
    .split(/[^a-zA-Z0-9]+/)
    .filter(Boolean)
    .map((part) => `${part.charAt(0).toUpperCase()}${part.slice(1)}`)
    .join("");
}

function isHTTPMethod(value) {
  return ["get", "post", "put", "patch", "delete", "head", "options", "trace"].includes(value);
}

function isSuccessResponse(status) {
  return /^2\d\d$/.test(status);
}

function isMediaType(value) {
  return /^[A-Za-z0-9!#$&^_.+-]+\/[A-Za-z0-9!#$&^_.+-]+:$/.test(value);
}

function unquote(value) {
  return value.replace(/^["']|["']$/g, "");
}
