package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

/*
{
  "mcpServers": {
    "mcp-curl-with-docker" :{
      "command": "docker",
      "args": [
        "run",
        "--rm",
        "-i",
        "mcp-curl"
      ]
    }
  }
}

*/

func main() {

	mcpClient, err := client.NewStdioMCPClient(
		"docker",
		[]string{}, // Empty ENV
		"run",
		"--rm",
		"-i",
		"mcp-curl",
	)
	if err != nil {
		log.Fatalf("😡 Failed to create client: %v", err)
	}
	defer mcpClient.Close()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize the client
	fmt.Println("🚀 Initializing mcp client...")
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "mcp-curl client 🌍",
		Version: "1.0.0",
	}

	initResult, err := mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}
	fmt.Printf(
		"🎉 Initialized with server: %s %s\n\n",
		initResult.ServerInfo.Name,
		initResult.ServerInfo.Version,
	)

	// List Tools
	fmt.Println("🛠️ Available tools...")
	toolsRequest := mcp.ListToolsRequest{}
	tools, err := mcpClient.ListTools(ctx, toolsRequest)
	if err != nil {
		log.Fatalf("😡 Failed to list tools: %v", err)
	}
	for _, tool := range tools.Tools {
		fmt.Printf("- %s: %s\n", tool.Name, tool.Description)
		fmt.Println("Arguments:", tool.InputSchema.Properties)

	}
	fmt.Println()


	// Fetch
	fmt.Println("📣 calling use_curl")
	fetchRequest := mcp.CallToolRequest{
		Request: mcp.Request{
			Method: "tools/call",
		},
	}
	fetchRequest.Params.Name = "use_curl"
	fetchRequest.Params.Arguments = map[string]interface{}{
		"url": "https://raw.githubusercontent.com/docker-sa/01-build-image/refs/heads/main/main.go",
	}


	result, err := mcpClient.CallTool(ctx, fetchRequest)
	if err != nil {
		log.Fatalf("😡 Failed to call the tool: %v", err)
	}
	// display the text content of result
	fmt.Println("🌍 content of the page:")
	fmt.Println(result.Content[0].(map[string]interface{})["text"])




}
