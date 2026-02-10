package mcpwrapper

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type Wrapper struct {
	server    *server.MCPServer
	validator *validator.Validate
}

type Handler func(ctx context.Context, args interface{}) (interface{}, error)

func New(mcpServer *server.MCPServer) *Wrapper {
	return &Wrapper{
		server:    mcpServer,
		validator: validator.New(),
	}
}

func (w *Wrapper) Register(name, description string, argsType interface{}, handler Handler) error {
	schema, err := buildSchema(argsType)
	if err != nil {
		return fmt.Errorf("failed to build schema for tool %s: %w", name, err)
	}

	tool := mcp.NewTool(name,
		mcp.WithDescription(description),
		mcp.WithString("input", mcp.Required(), mcp.Description("JSON-encoded input matching the schema")),
	)

	if schema != nil {
		tool.InputSchema = *schema
	}

	w.server.AddTool(tool, w.createHandler(argsType, handler))
	return nil
}

func (w *Wrapper) createHandler(argsType interface{}, handler Handler) server.ToolHandlerFunc {
	argType := reflect.TypeOf(argsType)
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		argsValue := reflect.New(argType).Interface()

		if err := coerceArgumentTypes(request.Params.Arguments, argType); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to coerce arguments: %v", err)), nil
		}

		if err := request.BindArguments(argsValue); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to bind arguments: %v", err)), nil
		}

		if err := w.validator.Struct(argsValue); err != nil {
			validationErr := formatValidationErrors(err)
			return mcp.NewToolResultError(validationErr.Error()), nil
		}

		result, err := handler(ctx, argsValue)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("handler error: %v", err)), nil
		}

		resultJSON, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}

		var resultMap map[string]interface{}
		if err := json.Unmarshal(resultJSON, &resultMap); err != nil {
			resultStr, ok := result.(string)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("failed to format result: %v", err)), nil
			}
			return mcp.NewToolResultText(resultStr), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	}
}

func coerceArgumentTypes(rawArgs any, argType reflect.Type) error {
	argsMap, ok := rawArgs.(map[string]any)
	if !ok || argsMap == nil {
		return nil
	}

	t := argType
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		key := fieldJSONKey(field)
		if key == "" {
			continue
		}

		value, exists := argsMap[key]
		if !exists {
			continue
		}

		strVal, ok := value.(string)
		if !ok {
			continue
		}

		if err := coerceValue(argsMap, key, field.Type, strVal); err != nil {
			return err
		}
	}

	return nil
}

func fieldJSONKey(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if tag != "" {
		name := strings.Split(tag, ",")[0]
		if name != "" {
			return name
		}
	}
	return field.Name
}

func coerceValue(args map[string]any, key string, fieldType reflect.Type, raw string) error {
	t := fieldType
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Bool:
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("field %s: %w", key, err)
		}
		args[key] = v
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bitSize := intBitSize(t.Kind())
		v, err := strconv.ParseInt(raw, 10, bitSize)
		if err != nil {
			return fmt.Errorf("field %s: %w", key, err)
		}
		switch t.Kind() {
		case reflect.Int:
			args[key] = int(v)
		case reflect.Int8:
			args[key] = int8(v)
		case reflect.Int16:
			args[key] = int16(v)
		case reflect.Int32:
			args[key] = int32(v)
		default:
			args[key] = v
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		bitSize := uintBitSize(t.Kind())
		v, err := strconv.ParseUint(raw, 10, bitSize)
		if err != nil {
			return fmt.Errorf("field %s: %w", key, err)
		}
		switch t.Kind() {
		case reflect.Uint:
			args[key] = uint(v)
		case reflect.Uint8:
			args[key] = uint8(v)
		case reflect.Uint16:
			args[key] = uint16(v)
		case reflect.Uint32:
			args[key] = uint32(v)
		case reflect.Uintptr:
			args[key] = uintptr(v)
		default:
			args[key] = v
		}
	case reflect.Float32, reflect.Float64:
		bitSize := 64
		if t.Kind() == reflect.Float32 {
			bitSize = 32
		}
		v, err := strconv.ParseFloat(raw, bitSize)
		if err != nil {
			return fmt.Errorf("field %s: %w", key, err)
		}
		if t.Kind() == reflect.Float32 {
			args[key] = float32(v)
		} else {
			args[key] = v
		}
	}
	return nil
}

func intBitSize(kind reflect.Kind) int {
	switch kind {
	case reflect.Int8:
		return 8
	case reflect.Int16:
		return 16
	case reflect.Int32:
		return 32
	case reflect.Int64:
		return 64
	default:
		return strconv.IntSize
	}
}

func uintBitSize(kind reflect.Kind) int {
	switch kind {
	case reflect.Uint8:
		return 8
	case reflect.Uint16:
		return 16
	case reflect.Uint32:
		return 32
	case reflect.Uint64, reflect.Uintptr:
		return 64
	default:
		return strconv.IntSize
	}
}

func buildSchema(argsType interface{}) (*mcp.ToolInputSchema, error) {
	t := reflect.TypeOf(argsType)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("argsType must be a struct, got %s", t.Kind())
	}

	properties := make(map[string]interface{})
	required := make([]string, 0) // Initialize as empty slice, not nil (MCP requires 'required' field)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		jsonName := strings.Split(jsonTag, ",")[0]

		prop := make(map[string]interface{})
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		prop["type"] = inferType(fieldType)

		if (fieldType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Array) {
			prop["items"] = map[string]interface{}{
				"type": inferType(fieldType.Elem()),
			}
		}

		jsonSchemaTag := field.Tag.Get("jsonschema")
		if jsonSchemaTag != "" {
			parseJSONSchemaTag(jsonSchemaTag, prop, &required, jsonName)
		}

		validateTag := field.Tag.Get("validate")
		if validateTag != "" {
			if strings.Contains(validateTag, "required") {
				if !contains(required, jsonName) {
					required = append(required, jsonName)
				}
			}
		}

		properties[jsonName] = prop
	}

	schema := &mcp.ToolInputSchema{
		Type:       "object",
		Properties: properties,
		Required:   required, // Always set, even if empty (MCP protocol requirement)
	}

	return schema, nil
}

func inferType(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	default:
		return "string"
	}
}

func parseJSONSchemaTag(tag string, prop map[string]interface{}, required *[]string, fieldName string) {
	parts := strings.Split(tag, ",")
	var enumValues []string

	for _, part := range parts {
		part = strings.TrimSpace(part)

		if part == "required" {
			if !contains(*required, fieldName) {
				*required = append(*required, fieldName)
			}
			continue
		}

		if strings.HasPrefix(part, "description=") {
			desc := strings.TrimPrefix(part, "description=")
			prop["description"] = desc
			continue
		}

		if strings.HasPrefix(part, "enum=") {
			enumValue := strings.TrimPrefix(part, "enum=")
			enumValues = append(enumValues, enumValue)
			continue
		}

		if strings.HasPrefix(part, "minimum=") {
			minimum := strings.TrimPrefix(part, "minimum=")
			prop["minimum"] = parseNumber(minimum)
			continue
		}

		if strings.HasPrefix(part, "maximum=") {
			maximum := strings.TrimPrefix(part, "maximum=")
			prop["maximum"] = parseNumber(maximum)
			continue
		}

		if strings.HasPrefix(part, "minLength=") {
			minLen := strings.TrimPrefix(part, "minLength=")
			prop["minLength"] = parseNumber(minLen)
			continue
		}

		if strings.HasPrefix(part, "maxLength=") {
			maxLen := strings.TrimPrefix(part, "maxLength=")
			prop["maxLength"] = parseNumber(maxLen)
			continue
		}
	}

	if len(enumValues) > 0 {
		prop["enum"] = enumValues
	}
}

func parseNumber(s string) interface{} {
	var i int
	if _, err := fmt.Sscanf(s, "%d", &i); err == nil {
		return i
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
		return f
	}
	return s
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
