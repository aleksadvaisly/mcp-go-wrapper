package mcpwrapper

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
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

func TestBuildSchema(t *testing.T) {
	schema, err := buildSchema(TestArgs{})
	if err != nil {
		t.Fatalf("buildSchema failed: %v", err)
	}

	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got '%s'", schema.Type)
	}

	if len(schema.Properties) != 4 {
		t.Errorf("Expected 4 properties, got %d", len(schema.Properties))
	}

	nameProp, ok := schema.Properties["name"].(map[string]interface{})
	if !ok {
		t.Fatal("name property not found or invalid type")
	}

	if nameProp["type"] != "string" {
		t.Errorf("Expected name type 'string', got '%v'", nameProp["type"])
	}

	if nameProp["description"] != "Test name" {
		t.Errorf("Expected description 'Test name', got '%v'", nameProp["description"])
	}

	ageProp, ok := schema.Properties["age"].(map[string]interface{})
	if !ok {
		t.Fatal("age property not found or invalid type")
	}

	if ageProp["type"] != "integer" {
		t.Errorf("Expected age type 'integer', got '%v'", ageProp["type"])
	}

	categoryProp, ok := schema.Properties["category"].(map[string]interface{})
	if !ok {
		t.Fatal("category property not found or invalid type")
	}

	enumValues, ok := categoryProp["enum"].([]string)
	if !ok {
		t.Fatal("category enum not found or invalid type")
	}

	if len(enumValues) != 3 {
		t.Errorf("Expected 3 enum values, got %d", len(enumValues))
	}

	if len(schema.Required) < 3 {
		t.Errorf("Expected at least 3 required fields, got %d", len(schema.Required))
	}
}

func TestInferType(t *testing.T) {
	tests := []struct {
		value    interface{}
		expected string
	}{
		{"string", "string"},
		{42, "integer"},
		{3.14, "number"},
		{true, "boolean"},
		{[]int{1, 2, 3}, "array"},
		{map[string]string{"key": "value"}, "object"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%T", tt.value), func(t *testing.T) {
			result := inferType(reflect.TypeOf(tt.value))
			if result != tt.expected {
				t.Errorf("Expected type '%s', got '%s'", tt.expected, result)
			}
		})
	}
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
	wrapper := New(mcpServer)

	handler := func(ctx context.Context, args interface{}) (interface{}, error) {
		a := args.(*TestArgs)
		return &TestResult{Message: fmt.Sprintf("Hello, %s", a.Name)}, nil
	}

	err := wrapper.Register("test-tool", "Test tool description", TestArgs{}, handler)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	tools := mcpServer.ListTools()
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool handler, got %d", len(tools))
	}
}

func TestRegisterCobra(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	wrapper := New(mcpServer)

	cmd := &cobra.Command{
		Use:   "test-cmd",
		Short: "Test command description",
	}

	handler := func(ctx context.Context, args interface{}) (interface{}, error) {
		return &TestResult{Message: "Success"}, nil
	}

	err := wrapper.RegisterCobra(cmd, TestArgs{}, handler)
	if err != nil {
		t.Fatalf("RegisterCobra failed: %v", err)
	}

	tools := mcpServer.ListTools()
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool handler, got %d", len(tools))
	}
}

func TestRegisterCobraNoUse(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	wrapper := New(mcpServer)

	cmd := &cobra.Command{
		Short: "Missing Use field",
	}

	handler := func(ctx context.Context, args interface{}) (interface{}, error) {
		return &TestResult{Message: "Success"}, nil
	}

	err := wrapper.RegisterCobra(cmd, TestArgs{}, handler)
	if err == nil {
		t.Error("Expected error for command without Use field")
	}
}

func TestHandlerInvocation(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	wrapper := New(mcpServer)

	expectedName := "TestUser"
	var receivedArgs *TestArgs

	handler := func(ctx context.Context, args interface{}) (interface{}, error) {
		receivedArgs = args.(*TestArgs)
		return &TestResult{Message: fmt.Sprintf("Hello, %s", receivedArgs.Name)}, nil
	}

	err := wrapper.Register("test-tool", "Test tool", TestArgs{}, handler)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	tools := mcpServer.ListTools()
	tool := tools["test-tool"]
	if tool == nil {
		t.Fatal("Tool handler not registered")
	}

	requestArgs := map[string]interface{}{
		"name":     expectedName,
		"age":      30,
		"category": "A",
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "test-tool",
			Arguments: requestArgs,
		},
	}

	result, err := tool.Handler(context.Background(), request)
	if err != nil {
		t.Fatalf("Handler invocation failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if receivedArgs == nil {
		t.Fatal("Handler was not called")
	}

	if receivedArgs.Name != expectedName {
		t.Errorf("Expected name '%s', got '%s'", expectedName, receivedArgs.Name)
	}
}

func TestHandlerValidationError(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	wrapper := New(mcpServer)

	handler := func(ctx context.Context, args interface{}) (interface{}, error) {
		t.Fatal("Handler should not be called with invalid args")
		return nil, nil
	}

	err := wrapper.Register("test-tool", "Test tool", TestArgs{}, handler)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	tools := mcpServer.ListTools()
	tool := tools["test-tool"]
	if tool == nil {
		t.Fatal("Tool handler not registered")
	}

	requestArgs := map[string]interface{}{
		"name":     "AB",
		"age":      30,
		"category": "A",
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "test-tool",
			Arguments: requestArgs,
		},
	}

	result, err := tool.Handler(context.Background(), request)
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

func TestHandlerError(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	wrapper := New(mcpServer)

	expectedError := fmt.Errorf("handler error")
	handler := func(ctx context.Context, args interface{}) (interface{}, error) {
		return nil, expectedError
	}

	err := wrapper.Register("test-tool", "Test tool", TestArgs{}, handler)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	tools := mcpServer.ListTools()
	tool := tools["test-tool"]
	if tool == nil {
		t.Fatal("Tool handler not registered")
	}

	requestArgs := map[string]interface{}{
		"name":     "ValidName",
		"age":      30,
		"category": "A",
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "test-tool",
			Arguments: requestArgs,
		},
	}

	result, err := tool.Handler(context.Background(), request)
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

func TestJSONMarshalResult(t *testing.T) {
	result := &TestResult{Message: "Test message"}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	var unmarshaled TestResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if unmarshaled.Message != result.Message {
		t.Errorf("Expected message '%s', got '%s'", result.Message, unmarshaled.Message)
	}
}
