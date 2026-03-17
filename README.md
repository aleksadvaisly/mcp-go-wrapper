# MCP Go Wrapper

Coercion + validation middleware for [mcp-go](https://github.com/mark3labs/mcp-go) tool handlers.

mcp-go v0.43+ handles schema generation (`mcp.WithInputSchema[T]()`) and typed argument binding (`request.BindArguments()`) natively. This wrapper sits between mcp-go and your handler to add two things LLMs need in practice:

1. **Bulk type coercion** -- LLMs routinely send `"20"` instead of `20`. The wrapper coerces string arguments to the types declared in your struct before mcp-go binds them.
2. **go-playground/validator runtime validation** -- `validate:"required,min=3,email"` tags checked after binding, before your handler runs.

## What It Adds Over Raw mcp-go

| Concern | Raw mcp-go | With wrapper |
|---|---|---|
| Schema generation | `mcp.WithInputSchema[T]()` | Same (delegates to mcp-go) |
| Argument binding | `request.BindArguments(&args)` | Same (delegates to mcp-go) |
| String-to-type coercion | None -- `"20"` fails to bind to `int` | Automatic before binding |
| Runtime validation | None -- write your own checks | `validate` struct tags |
| Error formatting | Raw errors | Human-readable validation messages |

## Installation

```bash
go get github.com/aleksadvaisly/mcp-go-wrapper
```

## Quick Start

Define your argument struct with `json` and `validate` tags:

```go
type GreetArgs struct {
    Name   string `json:"name"   validate:"required,min=1"`
    Format string `json:"format" validate:"omitempty,oneof=formal casual"`
}
```

Three ways to wire it up:

### 1. Convenience API

Schema generation + coercion + validation in one call. Best for most cases.

```go
mcpwrapper.Register[GreetArgs](w, "greet", "Greet someone",
    func(ctx context.Context, req mcp.CallToolRequest, args GreetArgs) (*mcp.CallToolResult, error) {
        return mcp.NewToolResultText("Hello " + args.Name), nil
    },
)
```

`Register` calls `mcp.WithInputSchema[T]()` for schema, then wraps your handler with `TypedHandler[T]` for coercion and validation.

### 2. Direct mcp-go Integration (Middleware Pattern)

Use `TypedHandler` directly with mcp-go's `AddTool`. Useful when you want full control over tool options.

```go
validate := validator.New()

mcpServer.AddTool(
    mcp.NewTool("greet",
        mcp.WithDescription("Greet someone"),
        mcp.WithInputSchema[GreetArgs](),
    ),
    mcpwrapper.TypedHandler[GreetArgs](validate, handler),
)
```

`TypedHandler` returns a `server.ToolHandlerFunc` that coerces, binds, validates, then calls your typed handler.

### 3. Structured Output

Returns `structuredContent` via `mcp.NewToolResultStructuredOnly`. Use when the MCP client expects machine-readable results.

```go
type CalcArgs struct {
    A  int    `json:"a"         validate:"required"`
    B  int    `json:"b"         validate:"required"`
    Op string `json:"operation" validate:"required,oneof=add subtract"`
}

type CalcResult struct {
    Result float64 `json:"result"`
}

mcpServer.AddTool(
    mcp.NewTool("calculate",
        mcp.WithDescription("Basic arithmetic"),
        mcp.WithInputSchema[CalcArgs](),
    ),
    mcpwrapper.StructuredHandler[CalcArgs, CalcResult](validate,
        func(ctx context.Context, req mcp.CallToolRequest, args CalcArgs) (CalcResult, error) {
            switch args.Op {
            case "add":
                return CalcResult{Result: float64(args.A + args.B)}, nil
            case "subtract":
                return CalcResult{Result: float64(args.A - args.B)}, nil
            default:
                return CalcResult{}, fmt.Errorf("unsupported operation: %s", args.Op)
            }
        },
    ),
)
```

## Validation Tags

Runtime validation uses [go-playground/validator](https://github.com/go-playground/validator). Tags go on the `validate` field tag.

| Tag | Description | Example |
|---|---|---|
| `required` | Field cannot be zero value | `validate:"required"` |
| `min=<n>` | Minimum length/value | `validate:"min=3"` |
| `max=<n>` | Maximum length/value | `validate:"max=50"` |
| `email` | Valid email format | `validate:"email"` |
| `url` | Valid URL format | `validate:"url"` |
| `oneof=<vals>` | Value must be one of list | `validate:"oneof=red blue green"` |
| `gte=<n>` | Greater than or equal | `validate:"gte=0"` |
| `lte=<n>` | Less than or equal | `validate:"lte=100"` |
| `omitempty` | Skip validation if empty | `validate:"omitempty,email"` |

Combine tags with commas:

```go
Age int `json:"age" validate:"required,gte=0,lte=120"`
```

## Struct Tags

The wrapper uses two tag types:

- **`json`** -- Field name mapping. Standard Go JSON tags. Used by mcp-go's `BindArguments` and by the wrapper's coercion logic.
- **`validate`** -- Runtime validation rules. Processed by go-playground/validator after binding.

Schema generation (`jsonschema` tags, descriptions, enums, min/max constraints) is handled by mcp-go's `WithInputSchema[T]()`, which uses [invopop/jsonschema](https://github.com/invopop/jsonschema) under the hood.

**Schema patching: `omitempty` removes fields from `required`**

`invopop/jsonschema` marks all struct fields as `required` by default. This is wrong for optional fields -- MCP clients will reject calls missing those fields. The wrapper fixes this automatically when using `Register[T]` or `RegisterCobra[T]`.

Rules for which fields end up in `required`:

| Scenario | In `required`? | Why |
|---|---|---|
| `validate:"required,min=1"` | Yes | Explicit required |
| `validate:"omitempty,gte=1"` | No | Explicit omitempty |
| `validate:"omitempty,required"` | Yes | `required` wins over `omitempty` |
| `validate:"email"` (no omitempty) | Yes | No omitempty = stays required |
| `validate:"required_if=Mode adv"` | Yes | `required_if` is NOT `required` (exact match) |
| No `validate` tag at all | Yes | Only omitempty removes from required |
| `json:"field,omitempty"` | No | `invopop/jsonschema` respects json omitempty too |

Example:

```go
type SearchArgs struct {
    Query  string `json:"query"  validate:"required,min=1"`           // -> required
    Limit  int    `json:"limit"  validate:"omitempty,gte=1"`          // -> NOT required
    Offset int    `json:"offset" validate:"omitempty"`                 // -> NOT required
    Format string `json:"format" validate:"email"`                     // -> required (no omitempty)
    Mode   string `json:"mode"   validate:"required_if=Format json"`  // -> required (required_if != required)
}
```

Resulting schema `required: ["query", "format", "mode"]`.

When all fields are optional, the schema contains `required: []` (empty array, not null or missing). This is required by the MCP protocol.

This patching only applies when using `Register[T]` or `RegisterCobra[T]`. If you use `TypedHandler` directly with `mcp.NewTool`, you manage the schema yourself.

Example combining schema and validation tags:

```go
type CreateUserArgs struct {
    Email string `json:"email"    jsonschema:"description=User email"  validate:"required,email"`
    Age   int    `json:"age"      jsonschema:"minimum=0,maximum=120"   validate:"required,gte=0,lte=120"`
    Role  string `json:"role"     jsonschema:"enum=admin,enum=user"    validate:"required,oneof=admin user"`
}
```

Here `jsonschema` tags are read by mcp-go for the tool schema; `validate` tags are read by the wrapper at call time.

## Cobra Integration

`RegisterCobra` extracts the tool name from `cmd.Use` and the description from `cmd.Short` (falling back to `cmd.Long`).

```go
greetCmd := &cobra.Command{
    Use:   "greet",
    Short: "Greet someone by name",
}

mcpwrapper.RegisterCobra[GreetArgs](w, greetCmd,
    func(ctx context.Context, req mcp.CallToolRequest, args GreetArgs) (*mcp.CallToolResult, error) {
        return mcp.NewToolResultText("Hello " + args.Name), nil
    },
)
```

### Integration Pattern: The `serve` Command

When adding MCP support to an existing CLI application, create a new `serve` subcommand instead of modifying the main application:

```go
var serveCmd = &cobra.Command{
    Use:   "serve",
    Short: "Start MCP server",
    Run: func(cmd *cobra.Command, args []string) {
        log.SetOutput(os.Stderr)

        mcpServer := server.NewMCPServer(
            "my-app", "1.0.0",
            server.WithInstructions("Describe what your server does."),
        )
        w := mcpwrapper.New(mcpServer)

        mcpwrapper.RegisterCobra[MyArgs](w, myCmd, myHandler)

        if err := server.ServeStdio(mcpServer); err != nil {
            log.Fatal(err)
        }
    },
}
```

This keeps the CLI working normally while adding MCP capability:
- `./my-app command` -- runs as regular CLI
- `./my-app serve` -- starts MCP server for AI integration

## CRITICAL: stdout vs stderr

The MCP protocol uses stdio (stdin/stdout) for JSON-RPC communication. Any non-protocol output to stdout corrupts the connection.

**DO** -- use stderr for all logging and debug output:
```go
log.SetOutput(os.Stderr)
fmt.Fprintln(os.Stderr, "message")
```

**DO NOT** -- write anything to stdout:
```go
fmt.Println("message")     // BREAKS PROTOCOL
log.Println("message")     // BREAKS PROTOCOL (default log writes to stderr, but verify)
fmt.Printf("debug: %v", x) // BREAKS PROTOCOL
```

MCP clients expect only valid JSON-RPC on stdout. If you mix in log lines:
```
Starting server...
{"jsonrpc":"2.0","id":1,"method":"tools/list"}
```
The client fails to parse and the connection breaks.

## CRITICAL: server.WithInstructions()

Always add `server.WithInstructions()` when creating the MCP server:

```go
mcpServer := server.NewMCPServer(
    "my-app", "1.0.0",
    server.WithInstructions("Describe your server's purpose and capabilities."),
)
```

Without instructions, AI agents will not understand what the server is for or when to use its tools. Instructions appear in the MCP `initialize` response and are how clients discover your server's capabilities.

## Architecture

```
+-------------------+
|  MCP Client       |  (Claude, Cursor, etc.)
|  (JSON-RPC)       |
+--------+----------+
         |
         v
+-------------------+
|  mcp-go           |  Schema, transport, binding
|  (protocol layer) |
+--------+----------+
         |
         v
+-------------------+
|  mcp-go-wrapper   |  <- Coercion + validation middleware
|  (this library)   |
+--------+----------+
         |
         v
+-------------------+
|  Your Handler     |  Receives typed, validated args
+-------------------+
```

The wrapper is a middleware layer. It does not replace or wrap the mcp-go server -- it wraps individual tool handler functions.

## Error Handling

### Validation Errors

Returned as tool errors (not Go errors) so the MCP client sees them:

```json
{
  "content": [{"type": "text", "text": "validation failed: Name: is required; Format: must be one of: formal casual"}],
  "isError": true
}
```

### Coercion Errors

```json
{
  "content": [{"type": "text", "text": "coercion failed: field age: strconv.ParseInt: parsing \"abc\": invalid syntax"}],
  "isError": true
}
```

### Handler Errors

For `StructuredHandler`, handler errors are wrapped as tool errors:

```json
{
  "content": [{"type": "text", "text": "handler error: division by zero"}],
  "isError": true
}
```

For `TypedHandler`, error handling is up to your handler since it returns `(*mcp.CallToolResult, error)` directly.

## API Reference

### Wrapper

```go
func New(mcpServer *server.MCPServer) *Wrapper
func NewWithValidator(mcpServer *server.MCPServer, v *validator.Validate) *Wrapper
```

`New` creates a wrapper with a default validator. `NewWithValidator` lets you pass a pre-configured `*validator.Validate` (e.g., with custom validation functions registered).

### Middleware Functions

```go
func TypedHandler[T any](v *validator.Validate, handler mcp.TypedToolHandlerFunc[T]) server.ToolHandlerFunc
func StructuredHandler[TArgs any, TResult any](v *validator.Validate, handler mcp.StructuredToolHandlerFunc[TArgs, TResult]) server.ToolHandlerFunc
```

Both return a `server.ToolHandlerFunc` that performs coercion -> binding -> validation -> your handler.

### Convenience Functions

```go
func Register[T any](w *Wrapper, name, description string, handler mcp.TypedToolHandlerFunc[T])
func RegisterCobra[T any](w *Wrapper, cmd *cobra.Command, handler mcp.TypedToolHandlerFunc[T]) error
```

`Register` calls `mcp.NewTool` with `WithInputSchema[T]()` and wraps the handler. `RegisterCobra` does the same but derives name and description from the Cobra command.

## For AI Agents

After reading this README, you should be able to autonomously integrate MCP support into a Go CLI application.

Steps:

1. **Analyze the target CLI** -- identify commands, their arguments, and business logic
2. **Create argument structs** -- define typed structs with `json`, `jsonschema`, and `validate` tags for each command
3. **Implement handlers** -- write `mcp.TypedToolHandlerFunc[T]` functions that call existing command logic
4. **Add a `serve` subcommand** -- create a new Cobra command that starts the MCP server (do not modify the main application entry point)
5. **Register tools** -- use `mcpwrapper.Register[T]()` or `mcpwrapper.RegisterCobra[T]()` to expose commands
6. **Set up the server** -- initialize with `server.WithInstructions()`, configure stdio transport, set `log.SetOutput(os.Stderr)`

Example `serve` command:

```go
var serveCmd = &cobra.Command{
    Use:   "serve",
    Short: "Start MCP server",
    Run: func(cmd *cobra.Command, cliArgs []string) {
        log.SetOutput(os.Stderr)

        mcpServer := server.NewMCPServer(
            "my-app", "1.0.0",
            server.WithInstructions("Describe your server here."),
        )
        w := mcpwrapper.New(mcpServer)

        mcpwrapper.Register[SearchArgs](w, "search", "Search documents", searchHandler)
        mcpwrapper.Register[CreateArgs](w, "create", "Create a document", createHandler)

        if err := server.ServeStdio(mcpServer); err != nil {
            log.Fatal(err)
        }
    },
}
```

Handler signature:

```go
func searchHandler(ctx context.Context, req mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, error) {
    results := doSearch(args.Query, args.Limit)
    return mcp.NewToolResultText(formatResults(results)), nil
}
```

## Dependencies

- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) -- MCP protocol implementation (v0.43+)
- [go-playground/validator](https://github.com/go-playground/validator) -- struct validation
- [spf13/cobra](https://github.com/spf13/cobra) -- CLI framework (optional, only needed for `RegisterCobra`)

## License

MIT

## Contributing

Contributions welcome. Please ensure:
- Tests pass (`go test ./...`)
- Code is formatted (`go fmt ./...`)
- Examples still work

## Credits

Built on top of the excellent [mcp-go](https://github.com/mark3labs/mcp-go) library by Mark3 Labs.
