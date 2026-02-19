package mcpwrapper

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
)

func RegisterCobra[T any](w *Wrapper, cmd *cobra.Command, handler mcp.TypedToolHandlerFunc[T]) error {
	name := cmd.Use
	if name == "" {
		return fmt.Errorf("cobra command must have a Use field")
	}

	description := cmd.Short
	if description == "" {
		description = cmd.Long
	}
	if description == "" {
		description = fmt.Sprintf("Execute %s command", name)
	}

	Register[T](w, name, description, handler)
	return nil
}
