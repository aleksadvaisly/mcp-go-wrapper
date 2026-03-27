# Idle Shutdown Issue

Date: 2026-03-27

## Context

Problem observed during agent usage:

- after the MCP server is shut down because of a longer idle period, the agent does not reconnect automatically
- the MCP entry becomes effectively dead until it is reconnected manually from the agent side

This makes the current stdio lifecycle behavior operationally expensive for long-lived agent sessions.

## What Was Checked

The repository no longer has a local `../mcp-go` checkout available, so the inspection was done against the actual module version resolved by Go:

- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4`

This is the version currently used by this project via `replace`.

## Findings

### 1. Idle shutdown is enabled by default for stdio servers

In `mcp-go`, stdio lifecycle management is built directly into `StdioServer`:

- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/server/stdio.go:411` sets `idleTimeout: time.Hour`
- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/server/stdio.go:412` sets `parentMonitor: true`

This means that plain `server.ServeStdio(...)` opts into both behaviors unless explicitly overridden.

### 2. Idle timeout only shuts the server down; it does not restart or reconnect it

The idle monitor cancels the stdio server context when the timeout expires:

- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/server/stdio.go:887`
- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/server/stdio.go:895`
- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/server/stdio.go:896`

The parent process monitor does the same when PPID changes:

- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/server/stdio.go:911`
- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/server/stdio.go:920`
- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/server/stdio.go:922`

There is no corresponding stdio-side restart or reconnection mechanism in this path. The behavior is cleanup-only.

### 3. Reconnection support is documented for SSE, not stdio

The `mcp-go` README explicitly mentions reconnection handling for SSE transport:

- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/README.md:653`

The stdio lifecycle section documents idle timeout and parent monitoring, but not any restart or reconnect semantics:

- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/README.md:655`
- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/README.md:659`
- `/Users/aleksander/go/pkg/mod/github.com/aleksadvaisly/mcp-go@v0.45.0-aleks.4/README.md:661`

### 4. This project currently uses the default stdio behavior

The example in this repository starts the MCP server with defaults:

- `examples/simple/main.go:103`

That means the current example path inherits:

- 1 hour idle timeout
- enabled parent monitor

unless the downstream server application overrides them explicitly.

## Assessment

The current design is coherent if the goal is preventing leaked stdio processes. It is not coherent if the goal is providing a stable MCP endpoint for long-running agent workflows.

The observed failure mode is therefore expected:

- stdio server shuts down after idle
- agent does not automatically respawn or reconnect it
- MCP integration becomes unavailable until manual reconnect

So the core issue is not that shutdown fails. The issue is that shutdown exists without a matching recovery mechanism in the stdio integration path.

## Recommended Options

### Option A: Disable idle shutdown for stdio

Short-term, simplest mitigation:

```go
server.ServeStdio(mcpServer, server.WithIdleTimeout(0))
```

This keeps the current stdio model and removes the behavior that breaks long-lived agent sessions.

### Option B: Keep parent monitor, disable idle timeout

This is likely the best near-term balance:

- orphaned processes are still cleaned up when the original launcher dies
- normal long idle periods no longer kill the MCP endpoint

Example:

```go
server.ServeStdio(
    mcpServer,
    server.WithIdleTimeout(0),
    server.WithParentProcessMonitor(true),
)
```

### Option C: Move lifecycle management above the MCP endpoint

Architecturally stronger option:

- keep a stable MCP server process alive
- let that process manage child worker processes with its own idle policy
- apply shutdown/restart semantics to workers, not to the MCP control plane itself

This matches the desired behavior better if the agent expects the MCP endpoint to remain continuously available.

## Wrapper Scope

`mcp-go-wrapper` currently looks like a coercion/validation layer, not a runtime supervisor.

Because of that, process orchestration probably should not be added directly to the wrapper unless the project is intentionally being expanded into a more opinionated runtime layer.

A cleaner split would be:

- keep `mcp-go-wrapper` focused on request/response shaping and validation
- add lifecycle supervision in a separate process or package

## Current Recommendation

If the objective is to remove current agent friction quickly, disable idle timeout for stdio-based usage first and reassess from there.

If the objective is to build a more durable multi-process architecture, design a stable broker MCP process and move worker lifecycle management behind it rather than inside stdio transport shutdown behavior.
