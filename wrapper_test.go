package mcpwrapper

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type TestArgs struct {
	Name     string `json:"name" jsonschema:"required,description=Test name" validate:"required,min=3"`
	Age      int    `json:"age" jsonschema:"required,minimum=0,maximum=120,description=Test age" validate:"required,gte=0,lte=120"`
	Email    string `json:"email" jsonschema:"description=Optional email" validate:"omitempty,email"`
	Category string `json:"category" jsonschema:"enum=A,enum=B,enum=C,description=Category" validate:"required,oneof=A B C"`
}

type TestResult struct {
	Message string `json:"message"`
}

func TestValidation(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	wrapper := New(mcpServer)

	tests := []struct {
		name      string
		args      interface{}
		shouldErr bool
	}{
		{
			name: "valid args",
			args: &TestArgs{
				Name:     "Alice",
				Age:      30,
				Email:    "alice@example.com",
				Category: "A",
			},
			shouldErr: false,
		},
		{
			name: "missing required field",
			args: &TestArgs{
				Age:      30,
				Category: "A",
			},
			shouldErr: true,
		},
		{
			name: "invalid email",
			args: &TestArgs{
				Name:     "Bob",
				Age:      25,
				Email:    "invalid-email",
				Category: "B",
			},
			shouldErr: true,
		},
		{
			name: "age out of range",
			args: &TestArgs{
				Name:     "Charlie",
				Age:      150,
				Category: "C",
			},
			shouldErr: true,
		},
		{
			name: "invalid category",
			args: &TestArgs{
				Name:     "Dave",
				Age:      40,
				Category: "D",
			},
			shouldErr: true,
		},
		{
			name: "name too short",
			args: &TestArgs{
				Name:     "Ed",
				Age:      35,
				Category: "A",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := wrapper.validator.Struct(tt.args)
			if tt.shouldErr && err == nil {
				t.Error("Expected validation error, got nil")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no validation error, got: %v", err)
			}
		})
	}
}

func TestFormatValidationErrors(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	wrapper := New(mcpServer)

	args := &TestArgs{
		Age:      150,
		Category: "D",
	}

	err := wrapper.validator.Struct(args)
	if err == nil {
		t.Fatal("Expected validation error")
	}

	formattedErr := formatValidationErrors(err)
	if formattedErr == nil {
		t.Fatal("Expected formatted error")
	}

	errMsg := formattedErr.Error()
	if errMsg == "" {
		t.Error("Expected non-empty error message")
	}

	if len(errMsg) < 10 {
		t.Error("Error message seems too short")
	}
}

func TestRegister(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[TestArgs](w, "test-tool", "Test tool description",
		func(ctx context.Context, req mcp.CallToolRequest, args TestArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(fmt.Sprintf("Hello, %s", args.Name)), nil
		},
	)

	tools := mcpServer.ListTools()
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}

	tool := tools["test-tool"]
	if tool == nil {
		t.Fatal("Tool 'test-tool' not found")
	}
}

func TestTypedHandler(t *testing.T) {
	v := validator.New()

	handler := TypedHandler[TestArgs](v,
		func(ctx context.Context, req mcp.CallToolRequest, args TestArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(fmt.Sprintf("name=%s age=%d", args.Name, args.Age)), nil
		},
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "test",
			Arguments: map[string]interface{}{
				"name":     "Alice",
				"age":      "30",
				"category": "A",
			},
		},
	}

	result, err := handler(context.Background(), request)
	if err != nil {
		t.Fatalf("TypedHandler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.IsError {
		t.Errorf("Expected success, got error result")
	}
}

func TestStructuredHandler(t *testing.T) {
	v := validator.New()

	handler := StructuredHandler[TestArgs, TestResult](v,
		func(ctx context.Context, req mcp.CallToolRequest, args TestArgs) (TestResult, error) {
			return TestResult{Message: fmt.Sprintf("Hello, %s", args.Name)}, nil
		},
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "test",
			Arguments: map[string]interface{}{
				"name":     "Alice",
				"age":      "30",
				"category": "A",
			},
		},
	}

	result, err := handler(context.Background(), request)
	if err != nil {
		t.Fatalf("StructuredHandler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.IsError {
		t.Errorf("Expected success, got error result")
	}
	if result.StructuredContent == nil {
		t.Error("Expected structured content, got nil")
	}
}

func TestTypedHandlerValidationError(t *testing.T) {
	v := validator.New()

	handler := TypedHandler[TestArgs](v,
		func(ctx context.Context, req mcp.CallToolRequest, args TestArgs) (*mcp.CallToolResult, error) {
			t.Fatal("Handler should not be called with invalid args")
			return nil, nil
		},
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "test",
			Arguments: map[string]interface{}{
				"name":     "AB",
				"age":      30,
				"category": "A",
			},
		},
	}

	result, err := handler(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected validation error in result, got error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if !result.IsError {
		t.Error("Expected error result for validation failure")
	}
}

func TestTypedHandlerCoercion(t *testing.T) {
	v := validator.New()

	var receivedAge int
	handler := TypedHandler[TestArgs](v,
		func(ctx context.Context, req mcp.CallToolRequest, args TestArgs) (*mcp.CallToolResult, error) {
			receivedAge = args.Age
			return mcp.NewToolResultText("ok"), nil
		},
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "test",
			Arguments: map[string]interface{}{
				"name":     "Alice",
				"age":      "30",
				"category": "A",
			},
		},
	}

	result, err := handler(context.Background(), request)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Expected success, got error result")
	}
	if receivedAge != 30 {
		t.Errorf("Expected coerced age 30, got %d", receivedAge)
	}
}

func TestHandlerInvocation(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	expectedName := "TestUser"
	var receivedArgs TestArgs

	Register[TestArgs](w, "test-tool", "Test tool",
		func(ctx context.Context, req mcp.CallToolRequest, args TestArgs) (*mcp.CallToolResult, error) {
			receivedArgs = args
			return mcp.NewToolResultText(fmt.Sprintf("Hello, %s", args.Name)), nil
		},
	)

	tools := mcpServer.ListTools()
	tool := tools["test-tool"]
	if tool == nil {
		t.Fatal("Tool handler not registered")
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "test-tool",
			Arguments: map[string]interface{}{
				"name":     expectedName,
				"age":      30,
				"category": "A",
			},
		},
	}

	result, err := tool.Handler(context.Background(), request)
	if err != nil {
		t.Fatalf("Handler invocation failed: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if receivedArgs.Name != expectedName {
		t.Errorf("Expected name '%s', got '%s'", expectedName, receivedArgs.Name)
	}
}

type SchemaTestArgs struct {
	Prompt  string `json:"prompt"  validate:"required,min=1"`
	Model   string `json:"model"   validate:"omitempty"`
	Session string `json:"session" validate:"omitempty"`
}

func TestRegisterOmitemptyNotRequired(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[SchemaTestArgs](w, "schema-test", "Schema test tool",
		func(ctx context.Context, req mcp.CallToolRequest, args SchemaTestArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)

	tools := mcpServer.ListTools()
	tool := tools["schema-test"]
	if tool == nil {
		t.Fatal("Tool 'schema-test' not found")
	}

	raw := tool.Tool.RawInputSchema
	if len(raw) == 0 {
		t.Fatal("Expected non-empty RawInputSchema")
	}

	var schema map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("Failed to unmarshal schema: %v", err)
	}

	reqRaw, ok := schema["required"]
	if !ok {
		t.Fatal("Expected 'required' key in schema")
	}

	var required []string
	if err := json.Unmarshal(reqRaw, &required); err != nil {
		t.Fatalf("Failed to unmarshal required: %v", err)
	}

	if len(required) != 1 {
		t.Errorf("Expected 1 required field, got %d: %v", len(required), required)
	}
	if len(required) > 0 && required[0] != "prompt" {
		t.Errorf("Expected required field 'prompt', got '%s'", required[0])
	}

	for _, r := range required {
		if r == "model" || r == "session" {
			t.Errorf("Field '%s' should not be in required (has omitempty)", r)
		}
	}
}

func TestRegisterAllRequiredFieldsKept(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[TestArgs](w, "all-required-test", "Test with mixed fields",
		func(ctx context.Context, req mcp.CallToolRequest, args TestArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)

	tools := mcpServer.ListTools()
	tool := tools["all-required-test"]
	if tool == nil {
		t.Fatal("Tool not found")
	}

	var schema map[string]json.RawMessage
	if err := json.Unmarshal(tool.Tool.RawInputSchema, &schema); err != nil {
		t.Fatalf("Failed to unmarshal schema: %v", err)
	}

	var required []string
	if err := json.Unmarshal(schema["required"], &required); err != nil {
		t.Fatalf("Failed to unmarshal required: %v", err)
	}

	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r] = true
	}

	if !requiredSet["name"] {
		t.Error("Expected 'name' in required")
	}
	if !requiredSet["age"] {
		t.Error("Expected 'age' in required")
	}
	if !requiredSet["category"] {
		t.Error("Expected 'category' in required")
	}
	if requiredSet["email"] {
		t.Error("'email' should not be in required (has omitempty)")
	}
}

func TestHandlerError(t *testing.T) {
	v := validator.New()

	handler := TypedHandler[TestArgs](v,
		func(ctx context.Context, req mcp.CallToolRequest, args TestArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultError("handler error"), nil
		},
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "test",
			Arguments: map[string]interface{}{
				"name":     "ValidName",
				"age":      30,
				"category": "A",
			},
		},
	}

	result, err := handler(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected handler error in result, got error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if !result.IsError {
		t.Error("Expected error result for handler failure")
	}
}
