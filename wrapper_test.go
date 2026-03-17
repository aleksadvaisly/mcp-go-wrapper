package mcpwrapper

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

type AllOptionalArgs struct {
	Provider string `json:"provider,omitempty" validate:"omitempty"`
	Space    string `json:"space,omitempty" validate:"omitempty"`
}

type NoValidateTagArgs struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type SingleFieldArgs struct {
	Query string `json:"query" validate:"required,min=1"`
}

type RequiredIfArgs struct {
	Mode  string `json:"mode"  validate:"required,oneof=simple advanced"`
	Query string `json:"query" validate:"required_if=Mode advanced"`
	Limit int    `json:"limit" validate:"omitempty,gte=1"`
}

type ConflictingTagArgs struct {
	Name string `json:"name" validate:"omitempty,required"`
}

type ValidateWithoutRequiredOrOmitemptyArgs struct {
	Email string `json:"email" validate:"email"`
	Age   int    `json:"age"   validate:"gte=0,lte=120"`
}

type JSONOmitemptyButValidateRequiredArgs struct {
	Name string `json:"name,omitempty" validate:"required,min=1"`
	Tag  string `json:"tag,omitempty"  validate:"omitempty"`
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

func TestRegisterKeepsEmptyRequiredArrayForAllOptionalFields(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[AllOptionalArgs](w, "all-optional-test", "Test with all optional fields",
		func(ctx context.Context, request mcp.CallToolRequest, args AllOptionalArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		})

	tools := mcpServer.ListTools()
	tool, ok := tools["all-optional-test"]
	if !ok {
		t.Fatal("Tool 'all-optional-test' not found")
	}

	var schema map[string]json.RawMessage
	if err := json.Unmarshal(tool.Tool.RawInputSchema, &schema); err != nil {
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

	if len(required) != 0 {
		t.Fatalf("Expected empty required array, got %v", required)
	}
}

func TestRegisterNoValidateTagFieldsStayRequired(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[NoValidateTagArgs](w, "no-validate-test", "Fields without validate tags",
		func(ctx context.Context, req mcp.CallToolRequest, args NoValidateTagArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)

	tools := mcpServer.ListTools()
	tool := tools["no-validate-test"]
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
		t.Error("'name' without validate tag should stay in required")
	}
	if !requiredSet["age"] {
		t.Error("'age' without validate tag should stay in required")
	}
}

func TestRegisterSingleRequiredField(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[SingleFieldArgs](w, "single-field-test", "Single required field",
		func(ctx context.Context, req mcp.CallToolRequest, args SingleFieldArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)

	tools := mcpServer.ListTools()
	tool := tools["single-field-test"]
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

	if len(required) != 1 || required[0] != "query" {
		t.Errorf("Expected required=[\"query\"], got %v", required)
	}
}

func TestRequiredArrayIsJSONArrayNotNull(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[AllOptionalArgs](w, "json-array-test", "Verify required serializes as []",
		func(ctx context.Context, req mcp.CallToolRequest, args AllOptionalArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)

	tools := mcpServer.ListTools()
	tool := tools["json-array-test"]
	if tool == nil {
		t.Fatal("Tool not found")
	}

	raw := string(tool.Tool.RawInputSchema)
	if !strings.Contains(raw, `"required":[]`) {
		t.Errorf("Expected required:[] in raw schema, got: %s", raw)
	}
	if strings.Contains(raw, `"required":null`) {
		t.Error("required must not be null")
	}
}

func TestRequiredIfDoesNotCountAsRequired(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[RequiredIfArgs](w, "required-if-test", "Test required_if edge case",
		func(ctx context.Context, req mcp.CallToolRequest, args RequiredIfArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)

	tools := mcpServer.ListTools()
	tool := tools["required-if-test"]
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

	if !requiredSet["mode"] {
		t.Error("'mode' with validate:\"required\" should be in required")
	}
	if requiredSet["limit"] {
		t.Error("'limit' with validate:\"omitempty\" should not be in required")
	}
	if !requiredSet["query"] {
		t.Error("'query' with validate:\"required_if=...\" should stay in required (required_if != required)")
	}
}

func TestConflictingOmitemptyAndRequired(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[ConflictingTagArgs](w, "conflict-test", "Test omitempty+required conflict",
		func(ctx context.Context, req mcp.CallToolRequest, args ConflictingTagArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)

	tools := mcpServer.ListTools()
	tool := tools["conflict-test"]
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
		t.Error("'name' with both omitempty and required should stay required (required wins)")
	}
}

func TestValidateWithoutRequiredOrOmitemptyStaysRequired(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[ValidateWithoutRequiredOrOmitemptyArgs](w, "plain-validate-test", "Test validate without required/omitempty",
		func(ctx context.Context, req mcp.CallToolRequest, args ValidateWithoutRequiredOrOmitemptyArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)

	tools := mcpServer.ListTools()
	tool := tools["plain-validate-test"]
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

	if !requiredSet["email"] {
		t.Error("'email' with validate:\"email\" (no omitempty) should stay in required")
	}
	if !requiredSet["age"] {
		t.Error("'age' with validate:\"gte=0,lte=120\" (no omitempty) should stay in required")
	}
}

func TestJSONOmitemptyAlsoExcludesFromRequired(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0")
	w := New(mcpServer)

	Register[JSONOmitemptyButValidateRequiredArgs](w, "json-omit-test", "Test json omitempty vs validate",
		func(ctx context.Context, req mcp.CallToolRequest, args JSONOmitemptyButValidateRequiredArgs) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)

	tools := mcpServer.ListTools()
	tool := tools["json-omit-test"]
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

	if requiredSet["name"] {
		t.Error("'name' with json:\"name,omitempty\" is excluded from required by invopop/jsonschema (json omitempty controls schema)")
	}
	if requiredSet["tag"] {
		t.Error("'tag' with validate:\"omitempty\" should not be in required")
	}
}
