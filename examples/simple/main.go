package main

import (
	"context"
	"fmt"
	"log"
	"os"

	mcpwrapper "github.com/aleksadvaisly/mcp-go-wrapper"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

type GreetArgs struct {
	Name   string `json:"name" jsonschema:"required,description=Name to greet" validate:"required,min=1"`
	Format string `json:"format" jsonschema:"enum=formal,enum=casual,description=Greeting style" validate:"omitempty,oneof=formal casual"`
}

type CalculateArgs struct {
	A        int    `json:"a" jsonschema:"required,description=First number" validate:"required"`
	B        int    `json:"b" jsonschema:"required,description=Second number" validate:"required"`
	Operation string `json:"operation" jsonschema:"required,enum=add,enum=subtract,enum=multiply,enum=divide,description=Operation to perform" validate:"required,oneof=add subtract multiply divide"`
}

type GreetResult struct {
	Message string `json:"message"`
}

type CalculateResult struct {
	Result float64 `json:"result"`
}

func greetHandler(ctx context.Context, args interface{}) (interface{}, error) {
	a := args.(*GreetArgs)

	var message string
	if a.Format == "formal" {
		message = fmt.Sprintf("Good day, %s", a.Name)
	} else {
		message = fmt.Sprintf("Hey %s!", a.Name)
	}

	return &GreetResult{Message: message}, nil
}

func calculateHandler(ctx context.Context, args interface{}) (interface{}, error) {
	a := args.(*CalculateArgs)

	var result float64
	switch a.Operation {
	case "add":
		result = float64(a.A + a.B)
	case "subtract":
		result = float64(a.A - a.B)
	case "multiply":
		result = float64(a.A * a.B)
	case "divide":
		if a.B == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		result = float64(a.A) / float64(a.B)
	}

	return &CalculateResult{Result: result}, nil
}

func main() {
	// CRITICAL: Set log output to stderr (stdout is reserved for MCP protocol)
	log.SetOutput(os.Stderr)

	mcpServer := server.NewMCPServer(
		"simple-example",
		"1.0.0",
	)

	wrapper := mcpwrapper.New(mcpServer)

	if err := wrapper.Register(
		"greet",
		"Greet someone by name with optional format",
		GreetArgs{},
		greetHandler,
	); err != nil {
		log.Fatalf("Failed to register greet tool: %v", err)
	}

	if err := wrapper.Register(
		"calculate",
		"Perform basic arithmetic operations",
		CalculateArgs{},
		calculateHandler,
	); err != nil {
		log.Fatalf("Failed to register calculate tool: %v", err)
	}

	greetCmd := &cobra.Command{
		Use:   "greet-cobra",
		Short: "Greet someone using Cobra command",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	if err := wrapper.RegisterCobra(greetCmd, GreetArgs{}, greetHandler); err != nil {
		log.Fatalf("Failed to register cobra command: %v", err)
	}

	log.Println("Starting MCP server...")
	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
