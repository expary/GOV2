package httpapi

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/expary/GOV2/internal/domain"
)

type openAPIOperation struct {
	Public               bool
	Summary              string
	Permission           string
	ParameterRefs        []string
	PathParams           []string
	DeclaredPathParams   []string
	OptionalPathParams   []string
	DeclaredQueryParams  []string
	PathParamTypes       map[string]string
	QueryParamTypes      map[string]string
	RequestBody          bool
	RequestBodyRequired  bool
	RequestMediaTypes    []string
	RequestSchema        string
	ResponseRefs         []string
	ResponseRefsByStatus map[string][]string
	Responses            map[string]struct{}
	SuccessMediaTypes    []string
	SuccessStatuses      []string
	SuccessSchema        string
	directParameter      *openAPIParameter
}

type openAPIParameter struct {
	Name     string
	In       string
	Required bool
	Type     string
}

type openAPIComponentSchema struct {
	Type        string
	Ref         string
	Description string
	Enum        []string
	Required    []string
	Properties  map[string]*openAPIComponentSchema
	Items       *openAPIComponentSchema
}

type openAPIComponentSchemaFrame struct {
	Kind   string
	Indent int
	Schema *openAPIComponentSchema
}

type openAPIComponentResponse struct {
	MediaTypes []string
	SchemaRefs []string
}

func TestOpenAPIContractMatchesRouteSpecs(t *testing.T) {
	operations := loadOpenAPIOperations(t)
	knownPermissions := systemPermissionCodes()
	seenRoutes := map[string]struct{}{}

	for _, route := range apiRouteSpecs() {
		key := openAPIKey(route.Method, route.Path)
		if _, exists := seenRoutes[key]; exists {
			t.Fatalf("duplicate route spec %s", key)
		}
		seenRoutes[key] = struct{}{}

		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}

		if route.Public {
			if !operation.Public {
				t.Fatalf("OpenAPI route %s must declare security: []", key)
			}
			if operation.Permission != "" {
				t.Fatalf("public OpenAPI route %s must not declare x-permission, got %q", key, operation.Permission)
			}
			continue
		}

		if operation.Public {
			t.Fatalf("protected OpenAPI route %s must not declare security: []", key)
		}
		if route.Permission == "" {
			if operation.Permission != "" {
				t.Fatalf("authenticated OpenAPI route %s must not declare x-permission, got %q", key, operation.Permission)
			}
			continue
		}
		if operation.Permission != route.Permission {
			t.Fatalf("OpenAPI route %s permission mismatch: got %q want %q", key, operation.Permission, route.Permission)
		}
		if _, ok := knownPermissions[route.Permission]; !ok {
			t.Fatalf("route %s uses unregistered permission %q", key, route.Permission)
		}
	}

	for key, operation := range operations {
		if _, ok := seenRoutes[key]; !ok {
			t.Fatalf("OpenAPI documents route %s but no HTTP route spec registers it", key)
		}
		if operation.Permission == "" {
			continue
		}
		if _, ok := knownPermissions[operation.Permission]; !ok {
			t.Fatalf("OpenAPI route %s uses unregistered permission %q", key, operation.Permission)
		}
	}
}

func TestOpenAPIConflictResponsesDocumentWriteConflicts(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for _, route := range apiRouteSpecs() {
		key := openAPIKey(route.Method, route.Path)
		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}
		if route.Conflict {
			if !containsString(operation.ResponseRefsByStatus["409"], "ConflictError") {
				t.Fatalf("OpenAPI route %s must document 409 ConflictError", key)
			}
			continue
		}
		if _, ok := operation.Responses["409"]; ok {
			t.Fatalf("OpenAPI route %s documents 409 but route spec has no conflict response", key)
		}
	}
}

func TestOpenAPINotFoundResponsesMatchRouteSpecs(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for _, route := range apiRouteSpecs() {
		key := openAPIKey(route.Method, route.Path)
		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}
		if route.NotFound {
			if !containsString(operation.ResponseRefsByStatus["404"], "NotFoundError") {
				t.Fatalf("OpenAPI route %s must document 404 NotFoundError", key)
			}
			continue
		}
		if _, ok := operation.Responses["404"]; ok {
			t.Fatalf("OpenAPI route %s documents 404 but route spec has no not-found response", key)
		}
	}
}

func TestOpenAPISecurityResponsesMatchRouteProtection(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for _, route := range apiRouteSpecs() {
		if route.Public {
			continue
		}
		key := openAPIKey(route.Method, route.Path)
		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}
		if !containsString(operation.ResponseRefsByStatus["401"], "UnauthorizedError") {
			t.Fatalf("OpenAPI protected route %s must document 401 UnauthorizedError", key)
		}
		if route.Permission == "" {
			continue
		}
		if !containsString(operation.ResponseRefsByStatus["403"], "ForbiddenError") {
			t.Fatalf("OpenAPI permission-protected route %s must document 403 ForbiddenError", key)
		}
	}
}

func TestOpenAPIRequestBodyResponsesDocumentBadRequests(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for key, operation := range operations {
		if !operation.RequestBody {
			continue
		}
		if _, ok := operation.Responses["400"]; !ok {
			t.Fatalf("OpenAPI request body route %s must document 400 bad request response", key)
		}
		if !containsString(operation.ResponseRefsByStatus["413"], "PayloadTooLargeError") {
			t.Fatalf("OpenAPI request body route %s must document 413 PayloadTooLargeError", key)
		}
		if !containsString(operation.ResponseRefsByStatus["415"], "UnsupportedMediaTypeError") {
			t.Fatalf("OpenAPI request body route %s must document 415 UnsupportedMediaTypeError", key)
		}
	}
}

func TestOpenAPIBadRequestResponsesMatchRouteSpecs(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for _, route := range apiRouteSpecs() {
		key := openAPIKey(route.Method, route.Path)
		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}

		refs := operation.ResponseRefsByStatus["400"]
		if route.ValidationError {
			if !containsString(refs, "ValidationError") {
				t.Fatalf("OpenAPI route %s must document 400 ValidationError", key)
			}
			continue
		}
		if containsString(refs, "ValidationError") {
			t.Fatalf("OpenAPI route %s documents 400 ValidationError but route spec has no field-validation response", key)
		}
		if route.RequestSchema != "" && !containsString(refs, "BadRequestError") {
			t.Fatalf("OpenAPI request body route %s without field validation must document 400 BadRequestError", key)
		}
	}
}

func TestOpenAPINonRequestBodyRoutesDoNotDocumentDecodeFailures(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for key, operation := range operations {
		if operation.RequestBody {
			continue
		}
		for _, status := range []string{"413", "415"} {
			if _, ok := operation.Responses[status]; ok {
				t.Fatalf("OpenAPI route %s documents %s JSON decode response without a request body", key, status)
			}
		}
	}
}

func TestOpenAPIRequestBodiesAreRequired(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for key, operation := range operations {
		if !operation.RequestBody {
			continue
		}
		if !operation.RequestBodyRequired {
			t.Fatalf("OpenAPI request body route %s must document required: true", key)
		}
	}
}

func TestOpenAPIRequestBodiesDocumentJSONContent(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for key, operation := range operations {
		if !operation.RequestBody {
			continue
		}
		if !containsString(operation.RequestMediaTypes, "application/json") {
			t.Fatalf("OpenAPI request body route %s must document application/json content", key)
		}
	}
}

func TestOpenAPIOperationsDocumentSuccessResponses(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for key, operation := range operations {
		if len(operation.SuccessStatuses) == 0 {
			t.Fatalf("OpenAPI route %s must document a 2xx success response", key)
		}
	}
}

func TestOpenAPIOperationsDocumentSummaries(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for _, route := range apiRouteSpecs() {
		key := openAPIKey(route.Method, route.Path)
		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}
		if route.Summary == "" {
			t.Fatalf("HTTP route spec %s must declare a summary", route.Method+" "+route.Path)
		}
		if operation.Summary == "" {
			t.Fatalf("OpenAPI route %s must document a non-empty summary", key)
		}
		if operation.Summary != route.Summary {
			t.Fatalf("OpenAPI route %s summary mismatch: got %q want %q", key, operation.Summary, route.Summary)
		}
	}
}

func TestOpenAPISuccessStatusesMatchRouteSpecs(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for _, route := range apiRouteSpecs() {
		key := openAPIKey(route.Method, route.Path)
		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}
		wantStatus := route.SuccessStatus
		if wantStatus == 0 {
			wantStatus = http.StatusOK
		}
		want := strconv.Itoa(wantStatus)
		if !containsString(operation.SuccessStatuses, want) {
			t.Fatalf("OpenAPI route %s success statuses %v must include %s", key, operation.SuccessStatuses, want)
		}
	}
}

func TestOpenAPIRequestBodiesDeclareSchemas(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for key, operation := range operations {
		if !operation.RequestBody {
			continue
		}
		if operation.RequestSchema == "" {
			t.Fatalf("OpenAPI request body route %s must declare a request schema", key)
		}
	}
}

func TestOpenAPIRequestBodiesMatchRouteSpecs(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for _, route := range apiRouteSpecs() {
		key := openAPIKey(route.Method, route.Path)
		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}
		if route.RequestSchema == "" {
			if operation.RequestBody {
				t.Fatalf("OpenAPI route %s declares requestBody but route spec has no request schema", key)
			}
			continue
		}
		if !operation.RequestBody {
			t.Fatalf("OpenAPI route %s must declare requestBody for route request schema %q", key, route.RequestSchema)
		}
		if operation.RequestSchema != route.RequestSchema {
			t.Fatalf("OpenAPI route %s request schema mismatch: got %q want %q", key, operation.RequestSchema, route.RequestSchema)
		}
	}
}

func TestOpenAPIRequestBodySchemasExist(t *testing.T) {
	operations := loadOpenAPIOperations(t)
	schemas := loadOpenAPIComponentSchemas(t)

	for key, operation := range operations {
		if operation.RequestSchema == "" {
			continue
		}
		if _, ok := schemas[operation.RequestSchema]; !ok {
			t.Fatalf("OpenAPI route %s references unknown request body schema %q", key, operation.RequestSchema)
		}
	}
}

func TestOpenAPISuccessResponseSchemasExist(t *testing.T) {
	operations := loadOpenAPIOperations(t)
	schemas := loadOpenAPIComponentSchemas(t)

	for key, operation := range operations {
		if operation.SuccessSchema == "" {
			continue
		}
		if _, ok := schemas[operation.SuccessSchema]; !ok {
			t.Fatalf("OpenAPI route %s references unknown success response schema %q", key, operation.SuccessSchema)
		}
	}
}

func TestOpenAPISuccessResponseSchemasMatchRouteSpecs(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for _, route := range apiRouteSpecs() {
		key := openAPIKey(route.Method, route.Path)
		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}
		if route.ResponseSchema == "" {
			if operation.SuccessSchema != "" {
				t.Fatalf("OpenAPI route %s declares success response schema %q but route spec has no response schema", key, operation.SuccessSchema)
			}
			continue
		}
		if operation.SuccessSchema != route.ResponseSchema {
			t.Fatalf("OpenAPI route %s success response schema mismatch: got %q want %q", key, operation.SuccessSchema, route.ResponseSchema)
		}
	}
}

func TestOpenAPINonTextSuccessResponsesDeclareSchemas(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for key, operation := range operations {
		if !openAPIRequiresSuccessResponseSchema(operation) {
			continue
		}
		if operation.SuccessSchema == "" {
			t.Fatalf("OpenAPI route %s non-text success response must declare a schema", key)
		}
	}
}

func TestOpenAPISuccessResponseMediaTypesAreSupported(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for key, operation := range operations {
		for _, mediaType := range operation.SuccessMediaTypes {
			if !isOpenAPISupportedSuccessMediaType(mediaType) {
				t.Fatalf("OpenAPI route %s uses unsupported success response media type %q", key, mediaType)
			}
		}
		if containsString(operation.SuccessMediaTypes, "text/plain") && len(operation.SuccessMediaTypes) != 1 {
			t.Fatalf("OpenAPI route %s text/plain success response must be the only success response media type", key)
		}
	}
}

func TestOpenAPIComponentSchemaRefsExist(t *testing.T) {
	schemas := loadOpenAPIComponentSchemas(t)
	refs := loadOpenAPIComponentSchemaRefs(t)

	for schemaName, refNames := range refs {
		for _, refName := range refNames {
			if _, ok := schemas[refName]; !ok {
				t.Fatalf("OpenAPI schema %s references unknown schema %q", schemaName, refName)
			}
		}
	}
}

func TestOpenAPIComponentResponseSchemaRefsExist(t *testing.T) {
	schemas := loadOpenAPIComponentSchemas(t)
	refs := loadOpenAPIComponentResponseSchemaRefs(t)

	for responseName, refNames := range refs {
		for _, refName := range refNames {
			if _, ok := schemas[refName]; !ok {
				t.Fatalf("OpenAPI response %s references unknown schema %q", responseName, refName)
			}
		}
	}
}

func TestOpenAPIErrorResponseComponentsUseJSONEnvelopes(t *testing.T) {
	responses := loadOpenAPIComponentResponses(t)
	wantSchemas := map[string]string{
		"BadRequestError":           "ErrorEnvelope",
		"UnauthorizedError":         "ErrorEnvelope",
		"ForbiddenError":            "ErrorEnvelope",
		"NotFoundError":             "ErrorEnvelope",
		"ConflictError":             "ErrorEnvelope",
		"PayloadTooLargeError":      "ErrorEnvelope",
		"UnsupportedMediaTypeError": "ErrorEnvelope",
		"ValidationError":           "ValidationErrorEnvelope",
	}

	for responseName, wantSchema := range wantSchemas {
		response, ok := responses[responseName]
		if !ok {
			t.Fatalf("OpenAPI components.responses.%s is missing", responseName)
		}
		if !containsString(response.MediaTypes, "application/json") {
			t.Fatalf("OpenAPI response %s must document application/json content", responseName)
		}
		if !containsString(response.SchemaRefs, wantSchema) {
			t.Fatalf("OpenAPI response %s must reference schema %s", responseName, wantSchema)
		}
	}
}

func TestOpenAPIComponentSchemaRequiredFieldsAreDeclared(t *testing.T) {
	schemas := loadOpenAPIComponentSchemaDefinitions(t)

	for _, schemaName := range sortedOpenAPIComponentSchemaNames(schemas) {
		assertOpenAPIRequiredFieldsDeclared(t, "components.schemas."+schemaName, schemas[schemaName])
	}
}

func TestOpenAPIComponentArraySchemasDeclareItems(t *testing.T) {
	schemas := loadOpenAPIComponentSchemaDefinitions(t)

	for _, schemaName := range sortedOpenAPIComponentSchemaNames(schemas) {
		assertOpenAPIArrayItemsDeclared(t, "components.schemas."+schemaName, schemas[schemaName])
	}
}

func TestOpenAPIComponentSchemaPropertiesGenerateConcreteFrontendTypes(t *testing.T) {
	schemas := loadOpenAPIComponentSchemaDefinitions(t)

	for _, schemaName := range sortedOpenAPIComponentSchemaNames(schemas) {
		assertOpenAPIConcreteFrontendTypes(t, "components.schemas."+schemaName, schemas[schemaName])
	}
}

func TestOpenAPIResponseRefsExist(t *testing.T) {
	operations := loadOpenAPIOperations(t)
	responses := loadOpenAPIComponentResponses(t)

	for key, operation := range operations {
		for _, refName := range operation.ResponseRefs {
			if _, ok := responses[refName]; !ok {
				t.Fatalf("OpenAPI route %s references unknown response %q", key, refName)
			}
		}
	}
}

func TestOpenAPIParameterRefsExist(t *testing.T) {
	operations := loadOpenAPIOperations(t)
	parameters := loadOpenAPIComponentParameters(t)

	for key, operation := range operations {
		for _, refName := range operation.ParameterRefs {
			if _, ok := parameters[refName]; !ok {
				t.Fatalf("OpenAPI route %s references unknown parameter %q", key, refName)
			}
		}
	}
}

func TestOpenAPIPathParametersMatchPaths(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for key, operation := range operations {
		for _, param := range operation.PathParams {
			if !containsString(operation.DeclaredPathParams, param) {
				t.Fatalf("OpenAPI route %s is missing path parameter declaration for %q", key, param)
			}
		}
		for _, param := range operation.DeclaredPathParams {
			if !containsString(operation.PathParams, param) {
				t.Fatalf("OpenAPI route %s declares path parameter %q that is not in the path", key, param)
			}
		}
		for _, param := range operation.OptionalPathParams {
			t.Fatalf("OpenAPI route %s path parameter %q must be required", key, param)
		}
	}
}

func TestOpenAPIPathAndQueryParametersDeclareSupportedSchemaTypes(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for key, operation := range operations {
		for _, param := range operation.DeclaredPathParams {
			assertOpenAPIParameterTypeSupported(t, key, "path", param, operation.PathParamTypes[param])
		}
		for _, param := range operation.DeclaredQueryParams {
			assertOpenAPIParameterTypeSupported(t, key, "query", param, operation.QueryParamTypes[param])
		}
	}
}

func TestOpenAPIQueryParametersMatchRouteSpecs(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for _, route := range apiRouteSpecs() {
		key := openAPIKey(route.Method, route.Path)
		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}
		assertSameStringSet(t, "OpenAPI query parameters for "+key, operation.DeclaredQueryParams, route.QueryParams)
	}
}

func TestOpenAPIPathAndQueryParameterTypesMatchRouteSpecs(t *testing.T) {
	operations := loadOpenAPIOperations(t)

	for _, route := range apiRouteSpecs() {
		key := openAPIKey(route.Method, route.Path)
		operation, ok := operations[key]
		if !ok {
			t.Fatalf("OpenAPI is missing route %s", key)
		}
		assertSameStringMap(t, "OpenAPI path parameter types for "+key, operation.PathParamTypes, route.PathParamTypes)
		assertSameStringMap(t, "OpenAPI query parameter types for "+key, operation.QueryParamTypes, route.QueryParamTypes)
	}
}

func TestOpenAPIGeneratedOperationNamesAreUnique(t *testing.T) {
	operations := loadOpenAPIOperations(t)
	keys := make([]string, 0, len(operations))
	for key := range operations {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	seenNames := map[string]string{}
	for _, key := range keys {
		parts := strings.SplitN(key, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid OpenAPI operation key %q", key)
		}
		name := openAPIGeneratedOperationName(parts[0], parts[1])
		if existingKey, ok := seenNames[name]; ok {
			t.Fatalf("OpenAPI route %s generates duplicate frontend operation name %q already used by %s", key, name, existingKey)
		}
		seenNames[name] = key
	}
}

func TestAPIContractDocumentListsBuiltInRoutesOnce(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "03-backend", "02-api-contract.md"))
	if err != nil {
		t.Fatalf("read API contract doc: %v", err)
	}

	routePattern := regexp.MustCompile("`(GET|POST|PUT|PATCH|DELETE) (/api/v1[^` ]*)`")
	documented := map[string]int{}
	for _, match := range routePattern.FindAllStringSubmatch(string(content), -1) {
		documented[match[1]+" "+match[2]]++
	}
	for key, count := range documented {
		if count > 1 {
			t.Fatalf("API contract doc documents route %s %d times", key, count)
		}
	}

	for _, route := range apiRouteSpecs() {
		key := route.Method + " " + route.Path
		if _, ok := documented[key]; !ok {
			t.Fatalf("API contract doc is missing route %s", key)
		}
	}
}

func systemPermissionCodes() map[string]struct{} {
	out := map[string]struct{}{}
	for _, permission := range domain.SystemPermissions() {
		out[permission.Code] = struct{}{}
	}
	return out
}

func loadOpenAPIOperations(t *testing.T) map[string]openAPIOperation {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("open OpenAPI spec: %v", err)
	}
	defer file.Close()

	operations := map[string]openAPIOperation{}
	scanner := bufio.NewScanner(file)
	var currentPath string
	var currentMethod string
	var currentSection string
	var currentRequestMediaType string
	var currentResponseStatus string
	var inRequestBody bool
	parameters := loadOpenAPIComponentParameters(t)

	flushDirectParameter := func() {
		if currentPath == "" || currentMethod == "" {
			return
		}
		key := currentMethod + " " + currentPath
		operation := operations[key]
		addOpenAPIParameter(&operation, operation.directParameter)
		operation.directParameter = nil
		operations[key] = operation
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "components:" {
			flushDirectParameter()
			currentPath = ""
			currentMethod = ""
			currentSection = ""
			currentRequestMediaType = ""
			currentResponseStatus = ""
			inRequestBody = false
			continue
		}
		if strings.HasPrefix(line, "  /") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":") {
			flushDirectParameter()
			currentPath = strings.TrimSuffix(trimmed, ":")
			currentMethod = ""
			currentSection = ""
			currentRequestMediaType = ""
			currentResponseStatus = ""
			inRequestBody = false
			continue
		}
		if currentPath == "" {
			continue
		}
		if currentMethod != "" && strings.HasPrefix(line, "      ") && !strings.HasPrefix(line, "        ") && strings.HasSuffix(trimmed, ":") {
			currentSection = strings.TrimSuffix(trimmed, ":")
			if currentSection != "requestBody" {
				currentRequestMediaType = ""
			}
			if currentSection != "responses" {
				currentResponseStatus = ""
			}
			inRequestBody = currentSection == "requestBody"
		}
		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.HasSuffix(trimmed, ":") {
			flushDirectParameter()
			method := strings.TrimSuffix(trimmed, ":")
			if !isOpenAPIHTTPMethod(method) {
				continue
			}
			currentMethod = strings.ToUpper(method)
			currentSection = ""
			currentRequestMediaType = ""
			currentResponseStatus = ""
			inRequestBody = false
			operations[currentMethod+" "+currentPath] = openAPIOperation{
				PathParams:           pathParamNames(currentPath),
				PathParamTypes:       map[string]string{},
				QueryParamTypes:      map[string]string{},
				Responses:            map[string]struct{}{},
				ResponseRefsByStatus: map[string][]string{},
			}
			continue
		}
		if currentMethod == "" || !strings.HasPrefix(line, "      ") {
			continue
		}

		key := currentMethod + " " + currentPath
		operation := operations[key]
		switch {
		case trimmed == "security: []":
			operation.Public = true
		case strings.HasPrefix(line, "      ") && !strings.HasPrefix(line, "        ") && strings.HasPrefix(trimmed, "summary:"):
			operation.Summary = openAPIValue(strings.TrimPrefix(trimmed, "summary:"))
		case strings.HasPrefix(trimmed, "x-permission:"):
			operation.Permission = strings.TrimSpace(strings.TrimPrefix(trimmed, "x-permission:"))
		case currentSection == "parameters" && strings.HasPrefix(trimmed, "- $ref:"):
			flushDirectParameter()
			ref := refName(strings.TrimPrefix(trimmed, "- "))
			operation = operations[key]
			operation.ParameterRefs = appendUnique(operation.ParameterRefs, ref)
			if parameter, ok := parameters[ref]; ok {
				addOpenAPIParameterValue(&operation, parameter)
			}
		case currentSection == "parameters" && strings.HasPrefix(trimmed, "- name:"):
			flushDirectParameter()
			operation = operations[key]
			operation.directParameter = &openAPIParameter{Name: strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:"))}
		case currentSection == "parameters" && strings.HasPrefix(trimmed, "in:") && operation.directParameter != nil:
			operation.directParameter.In = strings.TrimSpace(strings.TrimPrefix(trimmed, "in:"))
		case currentSection == "parameters" && strings.HasPrefix(trimmed, "required:") && operation.directParameter != nil:
			operation.directParameter.Required = strings.TrimSpace(strings.TrimPrefix(trimmed, "required:")) == "true"
		case currentSection == "parameters" && strings.HasPrefix(trimmed, "type:") && operation.directParameter != nil:
			operation.directParameter.Type = openAPIValue(strings.TrimPrefix(trimmed, "type:"))
		case currentSection == "requestBody" && strings.HasPrefix(trimmed, "required:"):
			operation.RequestBody = true
			operation.RequestBodyRequired = strings.TrimSpace(strings.TrimPrefix(trimmed, "required:")) == "true"
		case inRequestBody && strings.HasPrefix(trimmed, "$ref:"):
			if currentRequestMediaType != "application/json" {
				break
			}
			operation.RequestBody = true
			operation.RequestSchema = refName(trimmed)
		case currentSection == "requestBody" && strings.HasPrefix(line, "          ") && !strings.HasPrefix(line, "            ") && strings.HasSuffix(trimmed, ":") && isOpenAPIMediaType(trimmed):
			currentRequestMediaType = strings.Trim(strings.TrimSuffix(trimmed, ":"), `"`)
			operation.RequestMediaTypes = appendUnique(operation.RequestMediaTypes, currentRequestMediaType)
		case currentSection == "requestBody":
			operation.RequestBody = true
		case currentSection == "responses" && strings.HasPrefix(line, "          ") && !strings.HasPrefix(line, "            ") && strings.HasPrefix(trimmed, "$ref:"):
			ref := refName(trimmed)
			operation.ResponseRefs = appendUnique(operation.ResponseRefs, ref)
			if currentResponseStatus != "" {
				operation.ResponseRefsByStatus[currentResponseStatus] = appendUnique(operation.ResponseRefsByStatus[currentResponseStatus], ref)
			}
		case currentSection == "responses" && strings.HasPrefix(line, "        ") && !strings.HasPrefix(line, "          ") && strings.HasSuffix(trimmed, ":"):
			currentResponseStatus = strings.Trim(strings.TrimSuffix(trimmed, ":"), `"`)
			operation.Responses[currentResponseStatus] = struct{}{}
			if isOpenAPISuccessResponse(currentResponseStatus) {
				operation.SuccessStatuses = appendUnique(operation.SuccessStatuses, currentResponseStatus)
			}
		case currentSection == "responses" && isOpenAPISuccessResponse(currentResponseStatus) && strings.HasPrefix(line, "            ") && !strings.HasPrefix(line, "              ") && strings.HasSuffix(trimmed, ":") && isOpenAPIMediaType(trimmed):
			mediaType := strings.Trim(strings.TrimSuffix(trimmed, ":"), `"`)
			operation.SuccessMediaTypes = appendUnique(operation.SuccessMediaTypes, mediaType)
		case currentSection == "responses" && isOpenAPISuccessResponse(currentResponseStatus) && strings.HasPrefix(trimmed, "$ref:"):
			operation.SuccessSchema = refName(trimmed)
		}
		operations[key] = operation
	}
	flushDirectParameter()
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan OpenAPI spec: %v", err)
	}

	for key, operation := range operations {
		operation.directParameter = nil
		operations[key] = operation
	}
	return operations
}

func loadOpenAPIComponentSchemas(t *testing.T) map[string]struct{} {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("open OpenAPI spec: %v", err)
	}
	defer file.Close()

	schemas := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	inComponents := false
	inSchemas := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "components:":
			inComponents = true
			inSchemas = false
		case inComponents && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":"):
			inSchemas = strings.TrimSuffix(trimmed, ":") == "schemas"
		case inSchemas && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.HasSuffix(trimmed, ":"):
			schemas[strings.TrimSuffix(trimmed, ":")] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan OpenAPI spec: %v", err)
	}
	return schemas
}

func loadOpenAPIComponentSchemaDefinitions(t *testing.T) map[string]*openAPIComponentSchema {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("open OpenAPI spec: %v", err)
	}
	defer file.Close()

	schemas := map[string]*openAPIComponentSchema{}
	scanner := bufio.NewScanner(file)
	inComponents := false
	inSchemas := false
	currentName := ""
	currentSchema := (*openAPIComponentSchema)(nil)
	schemaFrames := []openAPIComponentSchemaFrame{}
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		switch {
		case trimmed == "components:":
			inComponents = true
			inSchemas = false
			currentName = ""
			currentSchema = nil
			schemaFrames = nil
		case inComponents && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":"):
			inSchemas = strings.TrimSuffix(trimmed, ":") == "schemas"
			currentName = ""
			currentSchema = nil
			schemaFrames = nil
		case inSchemas && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.HasSuffix(trimmed, ":"):
			currentName = strings.TrimSuffix(trimmed, ":")
			currentSchema = newOpenAPIComponentSchema()
			schemas[currentName] = currentSchema
			schemaFrames = []openAPIComponentSchemaFrame{{Kind: "node", Indent: 4, Schema: currentSchema}}
		case inSchemas && currentName != "" && currentSchema != nil:
			parseOpenAPIComponentSchemaLine(trimmed, openAPIIndent(line), &schemaFrames)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan OpenAPI spec: %v", err)
	}
	return schemas
}

func loadOpenAPIComponentSchemaRefs(t *testing.T) map[string][]string {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("open OpenAPI spec: %v", err)
	}
	defer file.Close()

	refs := map[string][]string{}
	scanner := bufio.NewScanner(file)
	inComponents := false
	inSchemas := false
	currentName := ""
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "components:":
			inComponents = true
			inSchemas = false
			currentName = ""
		case inComponents && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":"):
			inSchemas = strings.TrimSuffix(trimmed, ":") == "schemas"
			currentName = ""
		case inSchemas && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.HasSuffix(trimmed, ":"):
			currentName = strings.TrimSuffix(trimmed, ":")
		case inSchemas && currentName != "" && strings.Contains(trimmed, "$ref:"):
			refs[currentName] = appendUnique(refs[currentName], refName(trimmed))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan OpenAPI spec: %v", err)
	}
	return refs
}

func newOpenAPIComponentSchema() *openAPIComponentSchema {
	return &openAPIComponentSchema{
		Properties: map[string]*openAPIComponentSchema{},
	}
}

func parseOpenAPIComponentSchemaLine(trimmed string, indent int, frames *[]openAPIComponentSchemaFrame) {
	if frames == nil {
		return
	}
	for len(*frames) > 0 && indent <= (*frames)[len(*frames)-1].Indent {
		*frames = (*frames)[:len(*frames)-1]
	}
	if len(*frames) == 0 {
		return
	}
	frame := (*frames)[len(*frames)-1]
	switch {
	case frame.Kind == "required" && indent == frame.Indent+2 && strings.HasPrefix(trimmed, "- "):
		frame.Schema.Required = appendUnique(frame.Schema.Required, openAPIValue(strings.TrimPrefix(trimmed, "- ")))
		return
	case frame.Kind == "enum" && indent == frame.Indent+2 && strings.HasPrefix(trimmed, "- "):
		frame.Schema.Enum = appendUnique(frame.Schema.Enum, openAPIValue(strings.TrimPrefix(trimmed, "- ")))
		return
	case frame.Kind == "properties" && indent == frame.Indent+2 && strings.HasSuffix(trimmed, ":"):
		name := strings.TrimSuffix(trimmed, ":")
		property := newOpenAPIComponentSchema()
		frame.Schema.Properties[name] = property
		*frames = append(*frames, openAPIComponentSchemaFrame{Kind: "node", Indent: indent, Schema: property})
		return
	}
	if frame.Kind != "node" || indent != frame.Indent+2 {
		return
	}
	switch {
	case strings.HasPrefix(trimmed, "type:"):
		frame.Schema.Type = openAPIValue(strings.TrimPrefix(trimmed, "type:"))
	case strings.HasPrefix(trimmed, "$ref:"):
		frame.Schema.Ref = refName(trimmed)
	case strings.HasPrefix(trimmed, "description:"):
		frame.Schema.Description = openAPIValue(strings.TrimPrefix(trimmed, "description:"))
	case trimmed == "items:":
		frame.Schema.Items = newOpenAPIComponentSchema()
		*frames = append(*frames, openAPIComponentSchemaFrame{Kind: "node", Indent: indent, Schema: frame.Schema.Items})
	case trimmed == "required:":
		*frames = append(*frames, openAPIComponentSchemaFrame{Kind: "required", Indent: indent, Schema: frame.Schema})
	case trimmed == "properties:":
		*frames = append(*frames, openAPIComponentSchemaFrame{Kind: "properties", Indent: indent, Schema: frame.Schema})
	case trimmed == "enum:":
		*frames = append(*frames, openAPIComponentSchemaFrame{Kind: "enum", Indent: indent, Schema: frame.Schema})
	}
}

func assertOpenAPIRequiredFieldsDeclared(t *testing.T, label string, schema *openAPIComponentSchema) {
	t.Helper()
	if schema == nil {
		return
	}
	for _, field := range schema.Required {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("OpenAPI schema %s required field %q is not declared in properties", label, field)
		}
	}
	if schema.Items != nil {
		assertOpenAPIRequiredFieldsDeclared(t, label+".items", schema.Items)
	}
	for field, property := range schema.Properties {
		assertOpenAPIRequiredFieldsDeclared(t, label+"."+field, property)
	}
}

func assertOpenAPIArrayItemsDeclared(t *testing.T, label string, schema *openAPIComponentSchema) {
	t.Helper()
	if schema == nil {
		return
	}
	if schema.Type == "array" && schema.Items == nil {
		t.Fatalf("OpenAPI schema %s array must declare items", label)
	}
	if schema.Items != nil {
		assertOpenAPIArrayItemsDeclared(t, label+".items", schema.Items)
	}
	for field, property := range schema.Properties {
		assertOpenAPIArrayItemsDeclared(t, label+"."+field, property)
	}
}

func assertOpenAPIConcreteFrontendTypes(t *testing.T, label string, schema *openAPIComponentSchema) {
	t.Helper()
	if schema == nil {
		return
	}
	for field, property := range schema.Properties {
		propertyLabel := label + "." + field
		if openAPIJSDoctype(property) == "unknown" && !openAPIAllowsUnknownValue(property) {
			t.Fatalf("OpenAPI schema %s would generate unknown frontend type without an explicit arbitrary JSON description", propertyLabel)
		}
		assertOpenAPIConcreteFrontendTypes(t, propertyLabel, property)
	}
	if schema.Items != nil {
		if openAPIJSDocArrayItemType(schema.Items) == "unknown" && !openAPIAllowsUnknownValue(schema.Items) {
			t.Fatalf("OpenAPI schema %s.items would generate unknown frontend array item type without an explicit arbitrary JSON description", label)
		}
		assertOpenAPIConcreteFrontendTypes(t, label+".items", schema.Items)
	}
}

func openAPIJSDoctype(schema *openAPIComponentSchema) string {
	if schema == nil {
		return "unknown"
	}
	if schema.Ref != "" {
		return schema.Ref
	}
	if len(schema.Enum) > 0 {
		return "enum"
	}
	switch schema.Type {
	case "array":
		return "Array<" + openAPIJSDocArrayItemType(schema.Items) + ">"
	case "integer", "number":
		return "number"
	case "boolean":
		return "boolean"
	case "object":
		return "Record<string, unknown>"
	case "string":
		return "string"
	default:
		return "unknown"
	}
}

func openAPIJSDocArrayItemType(schema *openAPIComponentSchema) string {
	if schema == nil {
		return "unknown"
	}
	if schema.Ref != "" {
		return schema.Ref
	}
	switch schema.Type {
	case "integer", "number":
		return "number"
	case "boolean":
		return "boolean"
	case "string":
		return "string"
	case "object":
		if schema.Properties != nil {
			return "object"
		}
	}
	return "unknown"
}

func openAPIAllowsUnknownValue(schema *openAPIComponentSchema) bool {
	if schema == nil {
		return false
	}
	return regexp.MustCompile(`(?i)any valid json value`).MatchString(schema.Description)
}

func sortedOpenAPIComponentSchemaNames(schemas map[string]*openAPIComponentSchema) []string {
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func loadOpenAPIComponentParameters(t *testing.T) map[string]openAPIParameter {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("open OpenAPI spec: %v", err)
	}
	defer file.Close()

	parameters := map[string]openAPIParameter{}
	scanner := bufio.NewScanner(file)
	inComponents := false
	inParameters := false
	currentName := ""
	parameter := openAPIParameter{}
	flushParameter := func() {
		if currentName != "" && parameter.Name != "" && parameter.In != "" {
			parameters[currentName] = parameter
		}
		currentName = ""
		parameter = openAPIParameter{}
	}
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "components:":
			flushParameter()
			inComponents = true
			inParameters = false
		case inComponents && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":"):
			flushParameter()
			inParameters = strings.TrimSuffix(trimmed, ":") == "parameters"
		case inParameters && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.HasSuffix(trimmed, ":"):
			flushParameter()
			currentName = strings.TrimSuffix(trimmed, ":")
		case inParameters && currentName != "" && strings.HasPrefix(line, "      "):
			if strings.HasPrefix(trimmed, "name:") {
				parameter.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
			} else if strings.HasPrefix(trimmed, "in:") {
				parameter.In = strings.TrimSpace(strings.TrimPrefix(trimmed, "in:"))
			} else if strings.HasPrefix(trimmed, "required:") {
				parameter.Required = strings.TrimSpace(strings.TrimPrefix(trimmed, "required:")) == "true"
			} else if strings.HasPrefix(trimmed, "type:") {
				parameter.Type = openAPIValue(strings.TrimPrefix(trimmed, "type:"))
			}
		}
	}
	flushParameter()
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan OpenAPI spec: %v", err)
	}
	return parameters
}

func loadOpenAPIComponentResponses(t *testing.T) map[string]openAPIComponentResponse {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("open OpenAPI spec: %v", err)
	}
	defer file.Close()

	responses := map[string]openAPIComponentResponse{}
	scanner := bufio.NewScanner(file)
	inComponents := false
	inResponses := false
	currentName := ""
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "components:":
			inComponents = true
			inResponses = false
			currentName = ""
		case inComponents && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":"):
			inResponses = strings.TrimSuffix(trimmed, ":") == "responses"
			currentName = ""
		case inResponses && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.HasSuffix(trimmed, ":"):
			currentName = strings.TrimSuffix(trimmed, ":")
			responses[currentName] = openAPIComponentResponse{}
		case inResponses && currentName != "" && strings.HasSuffix(trimmed, ":") && isOpenAPIMediaType(trimmed):
			response := responses[currentName]
			response.MediaTypes = appendUnique(response.MediaTypes, strings.Trim(strings.TrimSuffix(trimmed, ":"), `"`))
			responses[currentName] = response
		case inResponses && currentName != "" && strings.Contains(trimmed, "$ref:"):
			response := responses[currentName]
			response.SchemaRefs = appendUnique(response.SchemaRefs, refName(trimmed))
			responses[currentName] = response
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan OpenAPI spec: %v", err)
	}
	return responses
}

func loadOpenAPIComponentResponseSchemaRefs(t *testing.T) map[string][]string {
	t.Helper()

	file, err := os.Open(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("open OpenAPI spec: %v", err)
	}
	defer file.Close()

	refs := map[string][]string{}
	scanner := bufio.NewScanner(file)
	inComponents := false
	inResponses := false
	currentName := ""
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "components:":
			inComponents = true
			inResponses = false
			currentName = ""
		case inComponents && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":"):
			inResponses = strings.TrimSuffix(trimmed, ":") == "responses"
			currentName = ""
		case inResponses && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.HasSuffix(trimmed, ":"):
			currentName = strings.TrimSuffix(trimmed, ":")
		case inResponses && currentName != "" && strings.Contains(trimmed, "$ref:"):
			refs[currentName] = appendUnique(refs[currentName], refName(trimmed))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan OpenAPI spec: %v", err)
	}
	return refs
}

func isOpenAPIHTTPMethod(value string) bool {
	switch value {
	case "get", "post", "put", "patch", "delete", "head", "options", "trace":
		return true
	default:
		return false
	}
}

func isOpenAPIMediaType(value string) bool {
	mediaType := strings.Trim(strings.TrimSuffix(value, ":"), `"`)
	return strings.Contains(mediaType, "/")
}

func isOpenAPISuccessResponse(status string) bool {
	return regexp.MustCompile(`^2\d\d$`).MatchString(status)
}

func isOpenAPISupportedSuccessMediaType(mediaType string) bool {
	return mediaType == "application/json" || mediaType == "text/plain"
}

func openAPIRequiresSuccessResponseSchema(operation openAPIOperation) bool {
	if len(operation.SuccessStatuses) == 0 {
		return false
	}
	allNoContent := true
	for _, status := range operation.SuccessStatuses {
		if status != "204" {
			allNoContent = false
			break
		}
	}
	if allNoContent {
		return false
	}
	return !(len(operation.SuccessMediaTypes) == 1 && operation.SuccessMediaTypes[0] == "text/plain")
}

func openAPIKey(method, routePath string) string {
	return method + " " + strings.TrimPrefix(routePath, "/api/v1")
}

func openAPIIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func openAPIValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func refName(line string) string {
	value := strings.TrimSpace(strings.TrimPrefix(line, "$ref:"))
	value = strings.Trim(value, `"'`)
	parts := strings.Split(value, "/")
	return parts[len(parts)-1]
}

func pathParamNames(path string) []string {
	names := []string{}
	for _, match := range regexp.MustCompile(`\{([^}]+)\}`).FindAllStringSubmatch(path, -1) {
		names = appendUnique(names, match[1])
	}
	return names
}

func openAPIGeneratedOperationName(method, path string) string {
	parts := []string{strings.ToLower(method)}
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		parts = append(parts, openAPIPascalCase(strings.NewReplacer("{", "", "}", "").Replace(part)))
	}
	return strings.Join(parts, "")
}

func openAPIPascalCase(value string) string {
	var builder strings.Builder
	for _, part := range regexp.MustCompile(`[^a-zA-Z0-9]+`).Split(value, -1) {
		if part == "" {
			continue
		}
		builder.WriteString(strings.ToUpper(part[:1]))
		builder.WriteString(part[1:])
	}
	return builder.String()
}

func addOpenAPIParameter(operation *openAPIOperation, parameter *openAPIParameter) {
	if operation == nil || parameter == nil || parameter.Name == "" {
		return
	}
	addOpenAPIParameterValue(operation, *parameter)
}

func addOpenAPIParameterValue(operation *openAPIOperation, parameter openAPIParameter) {
	if operation == nil || parameter.Name == "" {
		return
	}
	if parameter.In == "path" {
		operation.DeclaredPathParams = appendUnique(operation.DeclaredPathParams, parameter.Name)
		if _, exists := operation.PathParamTypes[parameter.Name]; !exists {
			operation.PathParamTypes[parameter.Name] = parameter.Type
		}
		if !parameter.Required {
			operation.OptionalPathParams = appendUnique(operation.OptionalPathParams, parameter.Name)
		}
	} else if parameter.In == "query" {
		operation.DeclaredQueryParams = appendUnique(operation.DeclaredQueryParams, parameter.Name)
		if _, exists := operation.QueryParamTypes[parameter.Name]; !exists {
			operation.QueryParamTypes[parameter.Name] = parameter.Type
		}
	}
}

func assertOpenAPIParameterTypeSupported(t *testing.T, routeKey, location, name, schemaType string) {
	t.Helper()
	if schemaType == "" {
		t.Fatalf("OpenAPI route %s %s parameter %q must declare a schema type", routeKey, location, name)
	}
	switch schemaType {
	case "integer", "number", "string", "boolean":
	default:
		t.Fatalf("OpenAPI route %s %s parameter %q uses unsupported schema type %q", routeKey, location, name, schemaType)
	}
}

func assertSameStringSet(t *testing.T, label string, got []string, want []string) {
	t.Helper()
	extra := setDifference(got, want)
	missing := setDifference(want, got)
	if len(extra) > 0 || len(missing) > 0 {
		t.Fatalf("%s mismatch: extra=%v missing=%v got=%v want=%v", label, extra, missing, got, want)
	}
}

func assertSameStringMap(t *testing.T, label string, got map[string]string, want map[string]string) {
	t.Helper()

	extra := setDifference(sortedStringMapKeys(got), sortedStringMapKeys(want))
	missing := setDifference(sortedStringMapKeys(want), sortedStringMapKeys(got))
	mismatched := []string{}
	for _, key := range sortedStringMapKeys(got) {
		wantValue, ok := want[key]
		if !ok {
			continue
		}
		if got[key] != wantValue {
			mismatched = append(mismatched, key+": got "+strconv.Quote(got[key])+" want "+strconv.Quote(wantValue))
		}
	}
	if len(extra) > 0 || len(missing) > 0 || len(mismatched) > 0 {
		t.Fatalf("%s mismatch: extra=%v missing=%v values=%v got=%v want=%v", label, extra, missing, mismatched, got, want)
	}
}

func setDifference(left []string, right []string) []string {
	out := []string{}
	for _, value := range left {
		if !containsString(right, value) {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func sortedStringMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}
