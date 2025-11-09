# MCP Go Wrapper

A lightweight, annotation-based library for wrapping Go CLI applications (especially Cobra-based) as [Model Context Protocol (MCP)](https://modelcontextprotocol.io) servers.

## Features

- **Struct Tag Support**: Define MCP tool schemas using standard Go struct tags (`json`, `jsonschema`, `validate`)
- **Automatic Validation**: Built-in argument validation using `go-playground/validator`
- **Cobra Integration**: Auto-register Cobra commands as MCP tools
- **Type Safety**: Direct function invocation with typed structs (no subprocess overhead)
- **Clean API**: Simple, idiomatic Go interface
- **Lightweight**: < 500 lines of core code

## Installation

```bash
go get github.com/aleksadvaisly/mcp-go-wrapper
```

## Recommended: Use with AI Coding Agents

This library is designed to be easily integrated into existing CLI applications by AI coding agents. The documentation is structured to enable autonomous implementation.

### For Users

If you have an existing CLI application and want to add MCP support, we recommend delegating the implementation to an AI coding agent:

```
"Please read the README.md from github.com/aleksadvaisly/mcp-go-wrapper and integrate MCP
support into my CLI application. Register all available commands as MCP tools with proper
argument schemas and validation."
```

### For AI Agents

After reading this README, you should be able to autonomously:

1. **Analyze the target CLI**: Identify existing commands, their arguments, and business logic
2. **Create argument structs**: Define typed structs with `json`, `jsonschema`, and `validate` tags for each command
3. **Implement handlers**: Wrap existing command logic in `Handler` functions that accept typed arguments
4. **Register tools**: Use `wrapper.Register()` or `wrapper.RegisterCobra()` to expose commands as MCP tools
5. **Setup server**: Initialize the MCP server and configure stdio transport

The API is intentionally minimal and follows Go idioms. All required interfaces are documented in the "API Documentation" section below.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    mcpwrapper "github.com/aleksadvaisly/mcp-go-wrapper"
    "github.com/mark3labs/mcp-go/server"
)

type GreetArgs struct {
    Name   string `json:"name"
                   jsonschema:"required,description=Name to greet"
                   validate:"required,min=1"`
    Format string `json:"format"
                   jsonschema:"enum=formal,enum=casual,description=Greeting style"
                   validate:"omitempty,oneof=formal casual"`
}

type GreetResult struct {
    Message string `json:"message"`
}

func main() {
    mcpServer := server.NewMCPServer(
        "my-app",
        "1.0.0",
    )

    wrapper := mcpwrapper.New(mcpServer)

    wrapper.Register(
        "greet",
        "Greet someone by name",
        GreetArgs{},
        func(ctx context.Context, args interface{}) (interface{}, error) {
            a := args.(*GreetArgs)

            message := fmt.Sprintf("Hey %s!", a.Name)
            if a.Format == "formal" {
                message = fmt.Sprintf("Good day, %s", a.Name)
            }

            return &GreetResult{Message: message}, nil
        },
    )

    log.Println("Starting MCP server...")
    if err := server.ServeStdio(mcpServer); err != nil {
        log.Fatalf("Server error: %v", err)
    }
}
```

## Struct Tag Reference

### JSON Tags (`json:"..."`)

Standard Go JSON tags for field naming:

```go
type Args struct {
    FieldName string `json:"field_name"`  // JSON key: "field_name"
}
```

### JSON Schema Tags (`jsonschema:"..."`)

Define MCP tool input schema:

| Tag | Description | Example |
|-----|-------------|---------|
| `required` | Mark field as required | `jsonschema:"required"` |
| `description=<text>` | Field description | `jsonschema:"description=User's email address"` |
| `enum=<value>` | Allowed values (repeat for multiple) | `jsonschema:"enum=small,enum=medium,enum=large"` |
| `minimum=<num>` | Minimum numeric value | `jsonschema:"minimum=0"` |
| `maximum=<num>` | Maximum numeric value | `jsonschema:"maximum=100"` |
| `minLength=<num>` | Minimum string length | `jsonschema:"minLength=3"` |
| `maxLength=<num>` | Maximum string length | `jsonschema:"maxLength=50"` |

Multiple tags can be combined with commas:

```go
Age int `json:"age" jsonschema:"required,minimum=0,maximum=120,description=User age in years"`
```

### Validation Tags (`validate:"..."`)

Runtime validation using [go-playground/validator](https://github.com/go-playground/validator):

| Tag | Description | Example |
|-----|-------------|---------|
| `required` | Field cannot be zero value | `validate:"required"` |
| `min=<n>` | Minimum length/value | `validate:"min=3"` |
| `max=<n>` | Maximum length/value | `validate:"max=50"` |
| `email` | Valid email format | `validate:"email"` |
| `url` | Valid URL format | `validate:"url"` |
| `oneof=<vals>` | Value must be one of list | `validate:"oneof=red blue green"` |
| `gte=<n>` | Greater than or equal | `validate:"gte=0"` |
| `lte=<n>` | Less than or equal | `validate:"lte=100"` |
| `omitempty` | Skip validation if empty | `validate:"omitempty,email"` |

Example combining all three tag types:

```go
type CreateUserArgs struct {
    Email    string `json:"email"
                     jsonschema:"required,description=User email address"
                     validate:"required,email"`
    Age      int    `json:"age"
                     jsonschema:"required,minimum=0,maximum=120,description=User age"
                     validate:"required,gte=0,lte=120"`
    Role     string `json:"role"
                     jsonschema:"enum=admin,enum=user,enum=guest,description=User role"
                     validate:"required,oneof=admin user guest"`
    Nickname string `json:"nickname"
                     jsonschema:"description=Optional display name"
                     validate:"omitempty,min=3,max=20"`
}
```

## API Documentation

### Creating a Wrapper

```go
func New(server *server.MCPServer) *Wrapper
```

Creates a new wrapper around an existing `mcp-go` server instance.

### Registering Tools

#### Direct Registration

```go
func (w *Wrapper) Register(
    name string,
    description string,
    argsType interface{},
    handler Handler,
) error
```

Register a tool with explicit name and description. The `argsType` should be an empty instance of your arguments struct.

#### Cobra Command Registration

```go
func (w *Wrapper) RegisterCobra(
    cmd *cobra.Command,
    argsType interface{},
    handler Handler,
) error
```

Auto-register from a Cobra command. Extracts name from `cmd.Use` and description from `cmd.Short` or `cmd.Long`.

Example:

```go
greetCmd := &cobra.Command{
    Use:   "greet",
    Short: "Greet someone by name",
}

wrapper.RegisterCobra(greetCmd, GreetArgs{}, greetHandler)
```

### Handler Function

```go
type Handler func(ctx context.Context, args interface{}) (interface{}, error)
```

Your handler receives:
- `ctx`: Context for cancellation and deadlines
- `args`: Pointer to your validated arguments struct

Return:
- `interface{}`: Any JSON-serializable result
- `error`: Error if operation failed

Example:

```go
func myHandler(ctx context.Context, args interface{}) (interface{}, error) {
    a := args.(*MyArgs)

    // Your logic here
    result := processData(a.Field1, a.Field2)

    return &MyResult{Output: result}, nil
}
```

## Complete Example

See [`examples/simple/main.go`](examples/simple/main.go) for a working example with multiple tools demonstrating:
- Basic tool registration
- Validation rules
- Enum handling
- Error handling
- Cobra command integration

To run the example:

```bash
cd examples/simple
go run main.go
```

## Error Handling

The wrapper provides clear error messages for common issues:

### Validation Errors

```json
{
  "error": "validation failed: Name: is required; Format: must be one of: formal casual"
}
```

### Handler Errors

```json
{
  "error": "handler error: division by zero"
}
```

### Schema Errors

Caught at registration time:

```
failed to build schema for tool xyz: argsType must be a struct, got string
```

## Architecture

```
┌─────────────────┐
│   Your CLI      │
│   (Cobra)       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  MCP Wrapper    │  ← Struct tags → Schema + Validation
│  (this library) │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   mcp-go        │  ← MCP Protocol
│   (transport)   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  MCP Client     │
│  (Claude, etc)  │
└─────────────────┘
```

## Design Principles

1. **Direct Invocation**: Handlers are called directly as Go functions, not via subprocess
2. **Type Safety**: Strong typing throughout with reflection only for schema generation
3. **Zero Magic**: What you see is what you get - no hidden code generation
4. **Standard Tags**: Uses familiar Go conventions (json, validate)
5. **Fail Fast**: Validation happens before handler invocation
6. **Clear Errors**: All errors include context for debugging

## Use Cases

- **CLI to API**: Expose your Cobra CLI as an MCP server for LLM integration
- **Development Tools**: Make dev tools accessible to AI assistants
- **Automation**: Bridge command-line utilities with AI workflows
- **Testing**: Validate tool schemas and argument handling

## Dependencies

- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) - MCP protocol implementation
- [go-playground/validator](https://github.com/go-playground/validator) - Struct validation
- [spf13/cobra](https://github.com/spf13/cobra) - CLI framework (optional, for `RegisterCobra`)

## Limitations

- JSON Schema generation is basic (covers common types)
- Complex nested structs may require manual schema definition
- Validation errors are formatted for clarity, not strict JSON Schema compliance
- Cobra integration doesn't automatically extract flag definitions (use struct tags instead)

## Future Improvements

- Support for custom JSON Schema generators
- Automatic flag extraction from Cobra commands
- Streaming response support
- Tool middleware/interceptors
- Enhanced schema generation for complex types

## License

MIT

## Contributing

Contributions welcome! Please ensure:
- Tests pass (`go test ./...`)
- Code is formatted (`go fmt`)
- Examples still work
- Documentation is updated

## Credits

Built on top of the excellent [mcp-go](https://github.com/mark3labs/mcp-go) library by Mark3 Labs.
