package mcp

import (
	"maps"
	"regexp"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/errors"
)

var curatedOperationIDs = []string{
	"MemoService_ListMemos",
	"MemoService_CreateMemo",
	"MemoService_GetMemo",
	"MemoService_UpdateMemo",
	"AuthService_GetCurrentUser",
}

type registeredOperation struct {
	OperationID string
	Operation   *openAPIOperation
	InputSchema jsonSchema
}

type requestBodySchemaOverride struct {
	// required replaces the base schema's required list when non-empty.
	required []string
	// clearRequired explicitly removes the base schema's required list.
	clearRequired     bool
	omittedProperties []string
	// minProperties, when > 0, requires the body to carry at least that many
	// properties. It replaces a cleared required list for partial updates so an
	// empty body is rejected up front instead of failing later at the API.
	minProperties int
}

// requestBodySchemaOverrides adjusts resource schemas to match how each HTTP
// binding consumes its request body. Resource-level required fields are too
// strict for partial updates, while body: "*" schemas include fields already
// supplied by the path.
var requestBodySchemaOverrides = map[string]requestBodySchemaOverride{
	"MemoService_CreateMemo": {
		required: []string{"content"},
	},
	"MemoService_UpdateMemo": {
		clearRequired:     true,
		omittedProperties: []string{"name"},
		minProperties:     1,
	},
}

var wordBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// validateOperationOverrides fails fast when a per-operation override table
// references an operation that is not in the registry (e.g. after a proto RPC
// rename). Without this, a stale key would silently miss and the renamed
// operation would lose its schema/annotation override with no error.
func validateOperationOverrides(registry map[string]*openAPIOperation) error {
	// An omitted property must exist in the body, or the override is stale
	// after a binding rename and silently leaves a path-bound field exposed.
	for operationID, override := range requestBodySchemaOverrides {
		operation, ok := registry[operationID]
		if !ok {
			return errors.Errorf("requestBodySchemaOverrides references unknown operation %q", operationID)
		}
		properties := schemaProperties(operation.RequestBodySchema["properties"])
		for _, name := range override.omittedProperties {
			if _, ok := properties[name]; !ok {
				return errors.Errorf("requestBodySchemaOverrides omits property %q missing from the %s request body", name, operationID)
			}
		}
	}
	return nil
}

func buildCuratedTools(registry map[string]*openAPIOperation) ([]*sdkmcp.Tool, map[string]*registeredOperation, error) {
	tools := make([]*sdkmcp.Tool, 0, len(curatedOperationIDs))
	operations := map[string]*registeredOperation{}
	for _, operationID := range curatedOperationIDs {
		operation, ok := registry[operationID]
		if !ok {
			return nil, nil, errors.Errorf("curated OpenAPI operation %q not found", operationID)
		}

		tool, registered := buildToolFromOperation(operation)
		if _, exists := operations[tool.Name]; exists {
			return nil, nil, errors.Errorf("duplicate MCP tool name %q", tool.Name)
		}

		tools = append(tools, tool)
		operations[tool.Name] = registered
	}

	if err := validateOperationOverrides(registry); err != nil {
		return nil, nil, err
	}
	return tools, operations, nil
}

func buildToolFromOperation(operation *openAPIOperation) (*sdkmcp.Tool, *registeredOperation) {
	name := toolNameFromOperationID(operation.OperationID)
	title := titleFromToolName(name)
	inputSchema := inputSchemaForOperation(operation)
	tool := &sdkmcp.Tool{
		Meta: sdkmcp.Meta{
			"operationId": operation.OperationID,
		},
		Name:         name,
		Title:        title,
		Description:  operation.Description,
		InputSchema:  inputSchema,
		OutputSchema: outputSchemaForOperation(operation),
		Annotations:  annotationsForOperation(operation, title),
	}

	return tool, &registeredOperation{
		OperationID: operation.OperationID,
		Operation:   operation,
		InputSchema: inputSchema,
	}
}

func toolNameFromOperationID(operationID string) string {
	service, method, ok := strings.Cut(operationID, "_")
	if !ok {
		return camelToSnake(operationID)
	}
	service = strings.TrimSuffix(service, "Service")
	return camelToSnake(service) + "_" + camelToSnake(method)
}

func camelToSnake(value string) string {
	return strings.ToLower(wordBoundary.ReplaceAllString(value, `${1}_${2}`))
}

func titleFromToolName(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func inputSchemaForOperation(operation *openAPIOperation) jsonSchema {
	properties := map[string]any{}
	required := []string{}
	defs := map[string]any{}
	for _, parameter := range operation.Parameters {
		schema := jsonSchema(sanitizeSchemaValue(parameter.Schema).(map[string]any))
		if parameter.Description != "" {
			schema["description"] = parameter.Description
		}
		properties[parameter.Name] = schema
		if parameter.Required {
			required = append(required, parameter.Name)
		}
	}

	if operation.RequestBody != nil {
		bodySchema := requestBodySchema(operation)
		maps.Copy(defs, extractSchemaDefs(bodySchema))
		properties["body"] = bodySchema
		if operation.RequestBody.Required {
			required = append(required, "body")
		}
	}

	schema := jsonSchema{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	if len(defs) > 0 {
		schema["$defs"] = defs
	}
	return schema
}

func requestBodySchema(operation *openAPIOperation) jsonSchema {
	if operation.RequestBodySchema == nil {
		return jsonSchema{"type": "object"}
	}
	schema := cloneSchema(operation.RequestBodySchema)
	override, ok := requestBodySchemaOverrides[operation.OperationID]
	if !ok {
		return schema
	}

	if override.clearRequired {
		delete(schema, "required")
	} else if len(override.required) > 0 {
		schema["required"] = override.required
	}

	if len(override.omittedProperties) > 0 {
		properties := maps.Clone(schemaProperties(schema["properties"]))
		for _, name := range override.omittedProperties {
			delete(properties, name)
		}
		schema["properties"] = properties
	}

	if override.minProperties > 0 {
		schema["minProperties"] = override.minProperties
	}
	return schema
}

func outputSchemaForOperation(operation *openAPIOperation) jsonSchema {
	if operation.ResponseSchema == nil {
		return okSchema()
	}
	return cloneSchema(operation.ResponseSchema)
}

func cloneSchema(schema jsonSchema) jsonSchema {
	clone := jsonSchema{}
	maps.Copy(clone, schema)
	return clone
}

func extractSchemaDefs(schema jsonSchema) map[string]any {
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		return nil
	}
	delete(schema, "$defs")
	return defs
}

// An update can overwrite user content even though it uses PATCH.
func annotationsForOperation(operation *openAPIOperation, title string) *sdkmcp.ToolAnnotations {
	annotations := annotationsForMethod(operation.Method, title)
	if operation.OperationID == "MemoService_UpdateMemo" {
		annotations.DestructiveHint = new(true)
	}
	return annotations
}

func annotationsForMethod(method string, title string) *sdkmcp.ToolAnnotations {
	openWorld := false
	destructive := false
	switch strings.ToUpper(method) {
	case "GET":
		return &sdkmcp.ToolAnnotations{
			Title:           title,
			ReadOnlyHint:    true,
			DestructiveHint: &destructive,
			IdempotentHint:  true,
			OpenWorldHint:   &openWorld,
		}
	case "DELETE":
		destructive = true
		return &sdkmcp.ToolAnnotations{
			Title:           title,
			ReadOnlyHint:    false,
			DestructiveHint: &destructive,
			IdempotentHint:  true,
			OpenWorldHint:   &openWorld,
		}
	default:
		return &sdkmcp.ToolAnnotations{
			Title:           title,
			ReadOnlyHint:    false,
			DestructiveHint: &destructive,
			IdempotentHint:  false,
			OpenWorldHint:   &openWorld,
		}
	}
}
