package mcpwrapper

import (
	"context"
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

func Register[T any](w *Wrapper, name, description string, handler mcp.TypedToolHandlerFunc[T]) {
	w.server.AddTool(
		mcp.NewTool(name, mcp.WithDescription(description), mcp.WithInputSchema[T]()),
		TypedHandler[T](w.validator, handler),
	)
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
