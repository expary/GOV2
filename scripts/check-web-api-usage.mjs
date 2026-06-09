import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const webSrcDir = path.join(rootDir, "web", "src");
const clientPath = path.join(webSrcDir, "api", "client.js");
const endpointPath = path.join(webSrcDir, "api", "endpoints.js");
const requestPath = path.join(webSrcDir, "api", "requests.js");
const typePath = path.join(webSrcDir, "api", "types.js");
const permissionPath = path.join(webSrcDir, "permissions.js");
const rawRequestPatterns = [
  { pattern: /\bfetch\s*\(/, name: "fetch(...)" },
  { pattern: /\bnew\s+Request\s*\(/, name: "new Request(...)" },
  { pattern: /\b(?:new\s+)?XMLHttpRequest\s*\(/, name: "XMLHttpRequest(...)" },
  { pattern: /\bnavigator\.sendBeacon\s*\(/, name: "navigator.sendBeacon(...)" },
  { pattern: /\b(?:new\s+)?EventSource\s*\(/, name: "EventSource(...)" },
  { pattern: /\b(?:new\s+)?WebSocket\s*\(/, name: "WebSocket(...)" },
  { pattern: /\baxios(?:\s*\(|\.[A-Za-z_$][\w$]*\s*\()/, name: "axios" },
];

const endpointContracts = readEndpointContracts(endpointPath);
const endpointMethods = readEndpointMethods(endpointPath);
const endpointNames = new Set(endpointMethods.keys());
const schemaNames = readSchemaNames(typePath);
const permissionContract = readPermissionContract(permissionPath);
const requestWrapperNames = readRequestWrapperNames(requestPath);
const requestWrapperEndpointCalls = readRequestWrapperEndpointCalls(requestPath);
const requestWrappersWithoutAbortSignal = readRequestWrappersWithoutAbortSignal(requestPath);
const errors = [];

for (const mismatch of readGeneratedRequestImportMismatches(requestPath)) {
  errors.push(mismatch);
}

for (const permission of readEndpointPermissions(endpointPath)) {
  if (!permissionContract.codes.has(permission)) {
    errors.push(`web/src/api/endpoints.js: unknown endpoint permission "${permission}"`);
  }
}

for (const schema of readEndpointBodySchemas(endpointPath)) {
  if (!schemaNames.has(schema)) {
    errors.push(`web/src/api/endpoints.js: unknown endpoint bodySchema "${schema}"`);
  }
}

for (const schema of readEndpointResponseSchemas(endpointPath)) {
  if (!schemaNames.has(schema)) {
    errors.push(`web/src/api/endpoints.js: unknown endpoint responseSchema "${schema}"`);
  }
}

for (const name of readApiEndpointReferences(requestPath)) {
  if (!endpointNames.has(name)) {
    errors.push(`web/src/api/requests.js: unknown apiEndpoints.${name}`);
  }
}

for (const name of requestWrapperNames) {
  if (!endpointNames.has(name)) {
    errors.push(`web/src/api/requests.js: unknown generated request wrapper "${name}"`);
  }
}

for (const name of endpointNames) {
  if (!requestWrapperNames.has(name)) {
    errors.push(`web/src/api/requests.js: missing generated request wrapper "${name}"`);
  }
}

for (const name of requestWrapperNames) {
  const endpointName = requestWrapperEndpointCalls.get(name);
  if (!endpointName) {
    errors.push(`web/src/api/requests.js: generated request wrapper ${name} must call apiRequest(apiEndpoints.${name}, options)`);
  } else if (endpointName !== name) {
    errors.push(`web/src/api/requests.js: generated request wrapper ${name} calls apiEndpoints.${endpointName} instead of apiEndpoints.${name}`);
  }
}

for (const name of requestWrappersWithoutAbortSignal) {
  errors.push(`web/src/api/requests.js: generated request wrapper ${name} must document options.signal as AbortSignal`);
}

for (const mismatch of readRequestWrapperJSDocMismatches(requestPath, endpointContracts)) {
  errors.push(mismatch);
}

for (const file of walk(webSrcDir)) {
  if (
    !/\.(js|vue)$/.test(file) ||
    file === clientPath ||
    file === endpointPath ||
    file === requestPath ||
    file === typePath ||
    file === permissionPath
  ) {
    continue;
  }

  const source = fs.readFileSync(file, "utf8");
  const relative = path.relative(rootDir, file);
  const clientModuleNames = moduleNamesForTarget(file, clientPath, ["@/api/client", "@/api/client.js"]);
  const requestModuleNames = moduleNamesForTarget(file, requestPath, ["@/api/requests", "@/api/requests.js"]);
  const endpointModuleNames = moduleNamesForTarget(file, endpointPath, ["@/api/endpoints", "@/api/endpoints.js"]);
  const permissionModuleNames = moduleNamesForTarget(file, permissionPath, ["@/permissions", "@/permissions.js"]);

  for (const match of source.matchAll(/\bapiEndpoints\.([A-Za-z_$][\w$]*)/g)) {
    const name = match[1];
    if (!endpointNames.has(name)) {
      errors.push(`${relative}: unknown apiEndpoints.${name}`);
      continue;
    }
    errors.push(`${relative}: use generated requests from @/api/requests instead of apiEndpoints.${name}`);
  }

  if (/\bapiRequest\s*\(/.test(source)) {
    errors.push(`${relative}: use generated requests from @/api/requests instead of apiRequest(...)`);
  }

  for (const imported of readNamedImportsFromModules(source, clientModuleNames)) {
    if (imported === "api" || imported === "apiRequest") {
      errors.push(`${relative}: do not import ${imported} outside web/src/api/requests.js`);
    }
  }

  if (
    hasNamespaceImportFromModules(source, clientModuleNames) ||
    hasDefaultImportFromModules(source, clientModuleNames) ||
    hasDynamicImportFromModules(source, clientModuleNames)
  ) {
    errors.push(`${relative}: do not import low-level API client transport outside web/src/api/requests.js`);
  }

  const requestImports = readNamedImportBindingsFromModules(source, requestModuleNames);
  for (const binding of requestImports) {
    if (!requestWrapperNames.has(binding.imported)) {
      errors.push(`${relative}: unknown generated request wrapper ${binding.imported}`);
    }
  }

  if (relative.startsWith("web/src/views/")) {
    for (const binding of requestImports) {
      if (endpointMethods.get(binding.imported) !== "GET") continue;
      for (const call of readFunctionCallArguments(source, binding.local)) {
        if (!callHasTopLevelSignalOption(call)) {
          errors.push(`${relative}: generated GET request ${binding.local}(...) in a view must pass an AbortSignal`);
        }
      }
    }
  }

  if (
    hasNamespaceImportFromModules(source, requestModuleNames) ||
    hasDefaultImportFromModules(source, requestModuleNames) ||
    hasDynamicImportFromModules(source, requestModuleNames)
  ) {
    errors.push(`${relative}: import generated API request wrappers by name from @/api/requests`);
  }

  if (hasStaticImportFromModules(source, endpointModuleNames) || hasDynamicImportFromModules(source, endpointModuleNames)) {
    errors.push(`${relative}: use generated requests from @/api/requests instead of @/api/endpoints`);
  }

  for (const { pattern, name } of rawRequestPatterns) {
    if (pattern.test(source)) {
      errors.push(`${relative}: use generated requests from @/api/requests instead of ${name}`);
    }
  }

  for (const objectName of readPermissionObjectImports(source, permissionModuleNames)) {
    const propertyPattern = new RegExp(`(^|[^.\\w$])${escapeRegExp(objectName)}\\.([A-Za-z_$][\\w$]*)`, "g");
    for (const match of source.matchAll(propertyPattern)) {
      const name = match[2];
      if (!permissionContract.names.has(name)) {
        errors.push(`${relative}: unknown ${objectName}.${name}`);
      }
    }
  }

  for (const match of source.matchAll(/["'`]([a-z][a-z0-9-]*:[a-z][a-z0-9-]*:[a-z][a-z0-9-]*|dashboard:view)["'`]/g)) {
    const permission = match[1];
    if (!permissionContract.codes.has(permission)) {
      errors.push(`${relative}: unknown permission literal "${permission}"`);
    }
  }

  if (/["'`]\*["'`]/.test(source)) {
    errors.push(`${relative}: use generated permissions.all instead of hardcoded wildcard permission "*"`);
  }

  if (/["'`]\/api\/v1\b/.test(source)) {
    errors.push(`${relative}: hardcoded /api/v1 path; use generated requests from @/api/requests`);
  }
}

if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exit(1);
}

console.log("web API endpoint, request wrapper, schema type, and permission usage is valid");

function readEndpointMethods(file) {
  return new Map(Array.from(readEndpointContracts(file), ([name, contract]) => [name, contract.method]));
}

function readEndpointContracts(file) {
  const source = fs.readFileSync(file, "utf8");
  return new Map(
    Array.from(source.matchAll(/^  ([A-Za-z_$][\w$]*): Object\.freeze\(\{ ([\s\S]*?) \}\),$/gm), (match) => [
      match[1],
      {
        method: readEndpointStringField(match[2], "method"),
        summary: readEndpointStringField(match[2], "summary"),
        body: readEndpointBooleanField(match[2], "body"),
        bodyRequired: readEndpointBooleanField(match[2], "bodyRequired"),
        bodySchema: readEndpointStringField(match[2], "bodySchema"),
        pathParams: readEndpointArrayField(match[2], "pathParams"),
        pathParamTypes: readEndpointObjectField(match[2], "pathParamTypes"),
        queryParams: readEndpointArrayField(match[2], "queryParams"),
        queryParamTypes: readEndpointObjectField(match[2], "queryParamTypes"),
        responseSchema: readEndpointStringField(match[2], "responseSchema"),
        responseType: readEndpointStringField(match[2], "responseType"),
      },
    ]),
  );
}

function readEndpointStringField(source, field) {
  const match = source.match(new RegExp(`\\b${field}: "([^"]+)"`));
  return match ? match[1] : "";
}

function readEndpointBooleanField(source, field) {
  return new RegExp(`\\b${field}: true\\b`).test(source);
}

function readEndpointArrayField(source, field) {
  const match = source.match(new RegExp(`\\b${field}: Object\\.freeze\\(\\[([^\\]]*)\\]\\)`));
  if (!match) return [];
  return Array.from(match[1].matchAll(/"([^"]+)"/g), (item) => item[1]);
}

function readEndpointObjectField(source, field) {
  const match = source.match(new RegExp(`\\b${field}: Object\\.freeze\\(\\{([^}]*)\\}\\)`));
  if (!match) return new Map();
  return new Map(Array.from(match[1].matchAll(/"([^"]+)":"([^"]+)"/g), (item) => [item[1], item[2]]));
}

function readEndpointPermissions(file) {
  const source = fs.readFileSync(file, "utf8");
  return new Set(Array.from(source.matchAll(/\bpermission: "([^"]+)"/g), (match) => match[1]));
}

function readApiEndpointReferences(file) {
  const source = fs.readFileSync(file, "utf8");
  return new Set(Array.from(source.matchAll(/\bapiEndpoints\.([A-Za-z_$][\w$]*)/g), (match) => match[1]));
}

function readRequestWrapperNames(file) {
  const source = fs.readFileSync(file, "utf8");
  return new Set(Array.from(source.matchAll(/\bexport\s+function\s+([A-Za-z_$][\w$]*)\s*\([^)]*\)/g), (match) => match[1]));
}

function readRequestWrapperEndpointCalls(file) {
  const source = fs.readFileSync(file, "utf8");
  return new Map(
    Array.from(
      source.matchAll(
        /\bexport\s+function\s+([A-Za-z_$][\w$]*)\s*\([^)]*\)\s*\{\s*return\s+apiRequest\(apiEndpoints\.([A-Za-z_$][\w$]*),\s*options\);\s*\}/g,
      ),
      (match) => [match[1], match[2]],
    ),
  );
}

function readRequestWrappersWithoutAbortSignal(file) {
  const source = fs.readFileSync(file, "utf8");
  return Array.from(source.matchAll(/\/\*\*([\s\S]*?)\*\/\s*export\s+function\s+([A-Za-z_$][\w$]*)\s*\(/g))
    .filter((match) => !match[1].includes("@param {AbortSignal} [options.signal]"))
    .map((match) => match[2]);
}

function readGeneratedRequestImportMismatches(file) {
  const source = fs.readFileSync(file, "utf8");
  const expectedImports = [
    'import { apiRequest } from "@/api/client";',
    'import { apiEndpoints } from "@/api/endpoints";',
  ];
  const errors = [];
  for (const expected of expectedImports) {
    if (!source.includes(expected)) {
      errors.push(`web/src/api/requests.js: generated request wrappers must include ${expected}`);
    }
  }
  for (const line of source.match(/^import .+;$/gm) || []) {
    if (!expectedImports.includes(line)) {
      errors.push(`web/src/api/requests.js: unexpected generated request import ${line}`);
    }
  }
  return errors;
}

function readRequestWrapperJSDocMismatches(file, contracts) {
  const source = fs.readFileSync(file, "utf8");
  const errors = [];
  for (const match of source.matchAll(/\/\*\*([\s\S]*?)\*\/\s*export\s+function\s+([A-Za-z_$][\w$]*)\s*\(([^)]*)\)/g)) {
    const contract = contracts.get(match[2]);
    if (!contract) continue;
    const comment = match[1];
    assertGeneratedSummaryJSDoc(errors, match[2], comment, contract);
    assertGeneratedOptionsJSDoc(errors, match[2], match[3], comment);
    assertGeneratedParameterJSDoc(errors, match[2], comment, "params", contract.pathParams, contract.pathParamTypes, false);
    assertGeneratedParameterJSDoc(errors, match[2], comment, "query", contract.queryParams, contract.queryParamTypes, true);
    assertGeneratedBodyJSDoc(errors, match[2], comment, contract);
    assertGeneratedReturnJSDoc(errors, match[2], comment, contract);
  }
  return errors;
}

function assertGeneratedSummaryJSDoc(errors, wrapperName, comment, contract) {
  if (!contract.summary) {
    errors.push(`web/src/api/endpoints.js: generated endpoint ${wrapperName} must include summary metadata`);
    return;
  }
  const expected = `* ${contract.summary}`;
  if (!comment.includes(expected)) {
    errors.push(`web/src/api/requests.js: generated request wrapper ${wrapperName} must document summary as ${expected}`);
  }
}

function assertGeneratedOptionsJSDoc(errors, wrapperName, signature, comment) {
  if (signature.trim() !== "options = {}") {
    errors.push(`web/src/api/requests.js: generated request wrapper ${wrapperName} must use options = {} default parameter`);
  }
  if (!comment.includes("@param {object} [options]")) {
    errors.push(`web/src/api/requests.js: generated request wrapper ${wrapperName} must document @param {object} [options]`);
  }
}

function assertGeneratedParameterJSDoc(errors, wrapperName, comment, optionName, fields, types, optional) {
  if (fields.length === 0) return;
  const expected = `@param {${parameterObjectType(fields, types, optional)}} ${optional ? `[options.${optionName}]` : `options.${optionName}`}`;
  if (!comment.includes(expected)) {
    errors.push(`web/src/api/requests.js: generated request wrapper ${wrapperName} must document ${optionName} as ${expected}`);
  }
}

function parameterObjectType(fields, types, optional) {
  const entries = fields.map((field) => `${field}${optional ? "?" : ""}: ${types.get(field) || "unknown"}`);
  return `{ ${entries.join(", ")} }`;
}

function assertGeneratedBodyJSDoc(errors, wrapperName, comment, contract) {
  const bodyLinePattern = /@param \{[^}]+\} (?:\[options\.body\]|options\.body)/;
  if (!contract.body) {
    if (bodyLinePattern.test(comment)) {
      errors.push(`web/src/api/requests.js: generated request wrapper ${wrapperName} must not document options.body`);
    }
    return;
  }
  const bodyName = contract.bodyRequired ? "options.body" : "[options.body]";
  const bodyType = contract.bodySchema ? `import("./types").${contract.bodySchema}` : "unknown";
  const expected = `@param {${bodyType}} ${bodyName}`;
  if (!comment.includes(expected)) {
    errors.push(`web/src/api/requests.js: generated request wrapper ${wrapperName} must document body as ${expected}`);
  }
}

function assertGeneratedReturnJSDoc(errors, wrapperName, comment, contract) {
  const returnType = generatedReturnType(contract);
  const expected = `@returns {Promise<${returnType}>}`;
  if (!comment.includes(expected)) {
    errors.push(`web/src/api/requests.js: generated request wrapper ${wrapperName} must document return as ${expected}`);
  }
}

function generatedReturnType(contract) {
  if (contract.responseType === "text") return "string";
  if (contract.responseSchema) return `import("./types").${contract.responseSchema}`;
  return "unknown";
}

function readNamedImportsFromModules(source, moduleNames) {
  return readNamedImportBindingsFromModules(source, moduleNames).map((binding) => binding.imported);
}

function readNamedImportBindingsFromModules(source, moduleNames) {
  return Array.from(moduleNames).flatMap((moduleName) => readNamedImportBindings(source, moduleName));
}

function readNamedImportBindings(source, moduleName) {
  const escapedModuleName = escapeRegExp(moduleName);
  const pattern = new RegExp(`\\bimport\\s*\\{([^;]*?)\\}\\s*from\\s*["']${escapedModuleName}["']`, "g");
  return Array.from(source.matchAll(pattern)).flatMap((match) =>
    match[1]
      .split(",")
      .map((part) => part.replace(/\/\*[\s\S]*?\*\//g, "").trim())
      .filter(Boolean)
      .map((part) => {
        const [imported, local] = part.split(/\s+as\s+/i).map((value) => value.trim());
        return { imported, local: local || imported };
      })
      .filter((binding) => binding.imported && binding.local),
  );
}

function readPermissionObjectImports(source, moduleNames) {
  return readNamedImportBindingsFromModules(source, moduleNames)
    .filter((binding) => binding.imported === "permissions")
    .map((binding) => binding.local);
}

function readFunctionCallArguments(source, functionName) {
  const calls = [];
  const pattern = new RegExp(`(^|[^.\\w$])${escapeRegExp(functionName)}\\s*\\(`, "g");
  for (const match of source.matchAll(pattern)) {
    let index = match.index + match[0].length;
    let depth = 1;
    let quote = "";
    let escaped = false;
    while (index < source.length && depth > 0) {
      const char = source[index];
      if (quote) {
        if (escaped) {
          escaped = false;
        } else if (char === "\\") {
          escaped = true;
        } else if (char === quote) {
          quote = "";
        }
      } else if (char === '"' || char === "'" || char === "`") {
        quote = char;
      } else if (char === "(") {
        depth += 1;
      } else if (char === ")") {
        depth -= 1;
      }
      index += 1;
    }
    if (depth === 0) {
      calls.push(source.slice(match.index + match[0].length, index - 1));
    }
  }
  return calls;
}

function callHasTopLevelSignalOption(call) {
  const firstArgument = topLevelParts(call, ",")[0]?.trim() || "";
  if (!firstArgument.startsWith("{") || !firstArgument.endsWith("}")) return false;
  const body = firstArgument.slice(1, -1);
  return topLevelParts(body, ",").some((property) => {
    const trimmed = property.trim();
    return trimmed === "signal" || /^signal\s*:/.test(trimmed);
  });
}

function topLevelParts(source, separator) {
  const parts = [];
  let start = 0;
  let roundDepth = 0;
  let squareDepth = 0;
  let braceDepth = 0;
  let quote = "";
  let escaped = false;
  for (let index = 0; index < source.length; index += 1) {
    const char = source[index];
    if (quote) {
      if (escaped) {
        escaped = false;
      } else if (char === "\\") {
        escaped = true;
      } else if (char === quote) {
        quote = "";
      }
      continue;
    }
    if (char === '"' || char === "'" || char === "`") {
      quote = char;
      continue;
    }
    if (char === "(") roundDepth += 1;
    if (char === ")") roundDepth -= 1;
    if (char === "[") squareDepth += 1;
    if (char === "]") squareDepth -= 1;
    if (char === "{") braceDepth += 1;
    if (char === "}") braceDepth -= 1;
    if (char === separator && roundDepth === 0 && squareDepth === 0 && braceDepth === 0) {
      parts.push(source.slice(start, index));
      start = index + 1;
    }
  }
  parts.push(source.slice(start));
  return parts;
}

function hasStaticImport(source, moduleName) {
  const escapedModuleName = escapeRegExp(moduleName);
  return new RegExp(`\\bimport\\s+(?:[^"'();]+?\\s+from\\s+)?["']${escapedModuleName}["']`).test(source);
}

function hasStaticImportFromModules(source, moduleNames) {
  return Array.from(moduleNames).some((moduleName) => hasStaticImport(source, moduleName));
}

function hasNamespaceImport(source, moduleName) {
  const escapedModuleName = escapeRegExp(moduleName);
  return new RegExp(`\\bimport\\s+\\*\\s+as\\s+[A-Za-z_$][\\w$]*\\s+from\\s*["']${escapedModuleName}["']`).test(source);
}

function hasNamespaceImportFromModules(source, moduleNames) {
  return Array.from(moduleNames).some((moduleName) => hasNamespaceImport(source, moduleName));
}

function hasDefaultImport(source, moduleName) {
  const escapedModuleName = escapeRegExp(moduleName);
  return new RegExp(
    `\\bimport\\s+[A-Za-z_$][\\w$]*(?:\\s*,\\s*(?:\\{[\\s\\S]*?\\}|\\*\\s+as\\s+[A-Za-z_$][\\w$]*))?\\s+from\\s*["']${escapedModuleName}["']`,
  ).test(source);
}

function hasDefaultImportFromModules(source, moduleNames) {
  return Array.from(moduleNames).some((moduleName) => hasDefaultImport(source, moduleName));
}

function hasDynamicImport(source, moduleName) {
  const escapedModuleName = escapeRegExp(moduleName);
  return new RegExp(`\\bimport\\s*\\(\\s*["']${escapedModuleName}["']\\s*\\)`).test(source);
}

function hasDynamicImportFromModules(source, moduleNames) {
  return Array.from(moduleNames).some((moduleName) => hasDynamicImport(source, moduleName));
}

function moduleNamesForTarget(fromFile, targetFile, aliases = []) {
  const relativeWithExtension = normalizeModulePath(path.relative(path.dirname(fromFile), targetFile));
  const relativeWithoutExtension = relativeWithExtension.replace(/\.js$/, "");
  return new Set([...aliases, relativeWithExtension, relativeWithoutExtension]);
}

function normalizeModulePath(value) {
  const normalized = value.split(path.sep).join("/");
  return normalized.startsWith(".") ? normalized : `./${normalized}`;
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function readEndpointBodySchemas(file) {
  const source = fs.readFileSync(file, "utf8");
  return new Set(Array.from(source.matchAll(/\bbodySchema: "([^"]+)"/g), (match) => match[1]));
}

function readEndpointResponseSchemas(file) {
  const source = fs.readFileSync(file, "utf8");
  return new Set(Array.from(source.matchAll(/\bresponseSchema: "([^"]+)"/g), (match) => match[1]));
}

function readSchemaNames(file) {
  const source = fs.readFileSync(file, "utf8");
  const names = new Set(Array.from(source.matchAll(/^  ([A-Za-z_$][\w$]*): Object\.freeze\(/gm), (match) => match[1]));
  if (names.size === 0) {
    throw new Error("web/src/api/types.js does not contain generated schema metadata");
  }
  return names;
}

function readPermissionContract(file) {
  const source = fs.readFileSync(file, "utf8");
  const entries = Array.from(
    source.matchAll(/^  ([A-Za-z_$][\w$]*): "([^"]+)",$/gm),
    (match) => ({
      name: match[1],
      code: match[2],
    }),
  );
  if (entries.length === 0) {
    throw new Error("web/src/permissions.js does not contain generated permission metadata");
  }
  return {
    names: new Set(entries.map((entry) => entry.name)),
    codes: new Set(entries.map((entry) => entry.code)),
  };
}

function walk(dir) {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  return entries.flatMap((entry) => {
    const fullPath = path.join(dir, entry.name);
    return entry.isDirectory() ? walk(fullPath) : [fullPath];
  });
}
