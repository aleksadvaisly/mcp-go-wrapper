package mcpwrapper

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func (w *Wrapper) RegisterCobra(cmd *cobra.Command, argsType interface{}, handler Handler) error {
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

	return w.Register(name, description, argsType, handler)
}

func (w *Wrapper) RegisterCobraCommand(cmd *cobra.Command, argsType interface{}) error {
	handler := func(ctx context.Context, args interface{}) (interface{}, error) {
		output := &struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: fmt.Sprintf("Command %s executed successfully", cmd.Use),
		}

		if cmd.RunE != nil {
			if err := cmd.RunE(cmd, []string{}); err != nil {
				output.Success = false
				output.Message = err.Error()
				return output, err
			}
		} else if cmd.Run != nil {
			cmd.Run(cmd, []string{})
		}

		return output, nil
	}

	return w.RegisterCobra(cmd, argsType, handler)
}
