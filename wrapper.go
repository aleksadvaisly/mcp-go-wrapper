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

func New(mcpServer *server.MCPServer) *Wrapper {
	return &Wrapper{
		server:    mcpServer,
		validator: validator.New(),
	}
}

func NewWithValidator(mcpServer *server.MCPServer, v *validator.Validate) *Wrapper {
	return &Wrapper{
		server:    mcpServer,
		validator: v,
	}
}

func TypedHandler[T any](v *validator.Validate, handler mcp.TypedToolHandlerFunc[T]) server.ToolHandlerFunc {
	argType := reflect.TypeOf((*T)(nil)).Elem()

	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := coerceArgumentTypes(request.Params.Arguments, argType); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("coercion failed: %v", err)), nil
		}

		var args T
		if err := request.BindArguments(&args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("binding failed: %v", err)), nil
		}

		if err := v.Struct(&args); err != nil {
			return mcp.NewToolResultError(formatValidationErrors(err).Error()), nil
		}

		return handler(ctx, request, args)
	}
}

func StructuredHandler[TArgs any, TResult any](v *validator.Validate, handler mcp.StructuredToolHandlerFunc[TArgs, TResult]) server.ToolHandlerFunc {
	argType := reflect.TypeOf((*TArgs)(nil)).Elem()

	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := coerceArgumentTypes(request.Params.Arguments, argType); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("coercion failed: %v", err)), nil
		}

		var args TArgs
		if err := request.BindArguments(&args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("binding failed: %v", err)), nil
		}

		if err := v.Struct(&args); err != nil {
			return mcp.NewToolResultError(formatValidationErrors(err).Error()), nil
		}

		result, err := handler(ctx, request, args)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("handler error: %v", err)), nil
		}

		return mcp.NewToolResultStructuredOnly(result), nil
	}
}

// RegisterLazy registers a tool with the default behaviour: the tool is subject
// to MCP tool-search deferral (the client loads its schema on demand). This is
// the canonical name; pair it with RegisterEager when the lazy-vs-eager intent
// should read clearly at the call site.
func RegisterLazy[T any](w *Wrapper, name, description string, handler mcp.TypedToolHandlerFunc[T]) {
	tool := mcp.NewTool(name, mcp.WithDescription(description), mcp.WithInputSchema[T]())
	patchRequired[T](&tool)
	w.server.AddTool(tool, TypedHandler[T](w.validator, handler))
}

// Register is a backwards-compatible alias for RegisterLazy.
func Register[T any](w *Wrapper, name, description string, handler mcp.TypedToolHandlerFunc[T]) {
	RegisterLazy(w, name, description, handler)
}

// RegisterEager is like Register but marks the tool as always-loaded, i.e.
// exempt from MCP tool-search deferral, via the anthropic/alwaysLoad _meta key
// honored by Claude Code (v2.1.121+). Reserve it for a small set of tools the
// client should see on every turn without a ToolSearch round-trip; each eager
// tool consumes context that would otherwise be free for the conversation.
func RegisterEager[T any](w *Wrapper, name, description string, handler mcp.TypedToolHandlerFunc[T]) {
	tool := mcp.NewTool(name, mcp.WithDescription(description), mcp.WithInputSchema[T]())
	patchRequired[T](&tool)
	tool.Meta = mcp.NewMetaFromMap(map[string]any{"anthropic/alwaysLoad": true})
	w.server.AddTool(tool, TypedHandler[T](w.validator, handler))
}

func patchRequired[T any](tool *mcp.Tool) {
	if len(tool.RawInputSchema) == 0 {
		return
	}

	var schema map[string]json.RawMessage
	if err := json.Unmarshal(tool.RawInputSchema, &schema); err != nil {
		return
	}

	var allRequired []string
	if reqRaw, ok := schema["required"]; ok {
		if err := json.Unmarshal(reqRaw, &allRequired); err != nil {
			return
		}
	}

	optional := optionalFields[T]()
	filtered := make([]string, 0)
	for _, name := range allRequired {
		if !optional[name] {
			filtered = append(filtered, name)
		}
	}

	b, err := json.Marshal(filtered)
	if err != nil {
		return
	}
	schema["required"] = b

	patched, err := json.Marshal(schema)
	if err != nil {
		return
	}
	tool.RawInputSchema = patched
}

func optionalFields[T any]() map[string]bool {
	t := reflect.TypeOf((*T)(nil)).Elem()
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	result := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		validate := field.Tag.Get("validate")
		if validate == "" {
			continue
		}
		if hasValidateRule(validate, "required") {
			continue
		}
		if hasValidateRule(validate, "omitempty") {
			key := fieldJSONKey(field)
			if key != "" {
				result[key] = true
			}
		}
	}
	return result
}

func hasValidateRule(tag, rule string) bool {
	for _, part := range strings.Split(tag, ",") {
		if part == rule {
			return true
		}
	}
	return false
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
