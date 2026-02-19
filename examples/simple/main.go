package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/go-playground/validator/v10"
	mcpwrapper "github.com/aleksadvaisly/mcp-go-wrapper"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

type GreetArgs struct {
	Name   string `json:"name" jsonschema:"required,description=Name to greet" validate:"required,min=1"`
	Format string `json:"format" jsonschema:"enum=formal,enum=casual,description=Greeting style" validate:"omitempty,oneof=formal casual"`
}

type CalculateArgs struct {
	A         int    `json:"a" jsonschema:"required,description=First number" validate:"required"`
	B         int    `json:"b" jsonschema:"required,description=Second number" validate:"required"`
	Operation string `json:"operation" jsonschema:"required,enum=add,enum=subtract,enum=multiply,enum=divide,description=Operation to perform" validate:"required,oneof=add subtract multiply divide"`
}

type GreetResult struct {
	Message string `json:"message"`
}

type CalculateResult struct {
	Result float64 `json:"result"`
}

func greetHandler(ctx context.Context, request mcp.CallToolRequest, args GreetArgs) (*mcp.CallToolResult, error) {
	var message string
	if args.Format == "formal" {
		message = fmt.Sprintf("Good day, %s", args.Name)
	} else {
		message = fmt.Sprintf("Hey %s!", args.Name)
	}

	return mcp.NewToolResultText(message), nil
}

func calculateHandler(ctx context.Context, request mcp.CallToolRequest, args CalculateArgs) (*mcp.CallToolResult, error) {
	var result float64
	switch args.Operation {
	case "add":
		result = float64(args.A + args.B)
	case "subtract":
		result = float64(args.A - args.B)
	case "multiply":
		result = float64(args.A * args.B)
	case "divide":
		if args.B == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		result = float64(args.A) / float64(args.B)
	}

	return mcp.NewToolResultText(fmt.Sprintf("%g", result)), nil
}

func main() {
	log.SetOutput(os.Stderr)

	mcpServer := server.NewMCPServer(
		"simple-example",
		"1.0.0",
		server.WithInstructions("A simple example MCP server demonstrating greetings and basic arithmetic operations. Helps language models generate personalized greetings and perform calculations."),
	)

	wrapper := mcpwrapper.New(mcpServer)

	// 1. Convenience API: Register greet tool via mcpwrapper.Register
	mcpwrapper.Register[GreetArgs](wrapper, "greet", "Greet someone by name with optional format", greetHandler)

	// 2. Native mcp-go + TypedHandler: Register calculate tool directly on mcpServer
	validate := validator.New()
	mcpServer.AddTool(
		mcp.NewTool("calculate",
			mcp.WithDescription("Perform basic arithmetic operations"),
			mcp.WithInputSchema[CalculateArgs](),
		),
		mcpwrapper.TypedHandler[CalculateArgs](validate, calculateHandler),
	)

	// 3. Cobra integration: Register greet-cobra via mcpwrapper.RegisterCobra
	greetCmd := &cobra.Command{
		Use:   "greet-cobra",
		Short: "Greet someone using Cobra command",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	if err := mcpwrapper.RegisterCobra[GreetArgs](wrapper, greetCmd, greetHandler); err != nil {
		log.Fatalf("Failed to register cobra command: %v", err)
	}

	log.Println("Starting MCP server...")
	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
