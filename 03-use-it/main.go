package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ollama/ollama/api"
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

	ctx := context.Background()

	var ollamaRawUrl string
	if ollamaRawUrl = os.Getenv("OLLAMA_HOST"); ollamaRawUrl == "" {
		ollamaRawUrl = "http://localhost:11434"
	}

	var chatLLM string
	if chatLLM = os.Getenv("CHAT_LLM"); chatLLM == "" {
		chatLLM = "qwen2.5-coder:3b"
	}

	var toolsLLM string
	if toolsLLM = os.Getenv("TOOLS_LLM"); toolsLLM == "" {
		//toolsLLM = "allenporter/xlam:1b"
		toolsLLM = "qwen2.5:0.5b"
	}

	url, _ := url.Parse(ollamaRawUrl)

	ollamaClient := api.NewClient(url, http.DefaultClient)

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

	// Define tool with Ollama format

	// From: https://github.com/mark3labs/mcphost/blob/main/pkg/llm/ollama/provider.go
	// Convert tools to Ollama format
	ollamaTools := ConvertToOllamaTools(tools.Tools)

	// Display the Ollama format
	fmt.Println("🦙 Ollama tools:")
	fmt.Println(ollamaTools)

	// Have a "tool chat" with Ollama 🦙
	// Prompt construction

	systemMCPInstructions := `You are a useful AI agent. 
	Your job is to understand the user prompt ans decide if you need to use a tool to run external commands.
	Ignore all things not related to the usage of a tool.
	`
	userInstructions := `Fetch this page: https://raw.githubusercontent.com/docker-sa/01-build-image/refs/heads/main/main.go 
	and then analyse the source code.
	`

	messages := []api.Message{
		{Role: "system", Content: systemMCPInstructions},
		{Role: "user", Content: userInstructions},
	}

	var FALSE = false
	req := &api.ChatRequest{
		Model:    toolsLLM,
		Messages: messages,
		Options: map[string]interface{}{
			"temperature":   0.0,
			"repeat_last_n": 2,
		},
		Tools:  ollamaTools,
		Stream: &FALSE,
	}

	contentForThePrompt := ""

	err = ollamaClient.Chat(ctx, req, func(resp api.ChatResponse) error {

		// Ollma found tool(s) to call
		for _, toolCall := range resp.Message.ToolCalls {

			fmt.Println("🦙🛠️", toolCall.Function.Name, toolCall.Function.Arguments)
			// 🖐️ Call the mcp server
			fmt.Println("📣 calling", toolCall.Function.Name)
			fetchRequest := mcp.CallToolRequest{
				Request: mcp.Request{
					Method: "tools/call",
				},
			}
			fetchRequest.Params.Name = toolCall.Function.Name
			fetchRequest.Params.Arguments = toolCall.Function.Arguments

			result, err := mcpClient.CallTool(ctx, fetchRequest)
			if err != nil {
				log.Fatalf("😡 Failed to call the tool: %v", err)
			}
			// display the text content of result
			fmt.Println("🌍 content of the result:")
			contentForThePrompt += result.Content[0].(map[string]interface{})["text"].(string)
			fmt.Println(contentForThePrompt)
		}

		return nil
	})

	if err != nil {
		log.Fatalln("😡", err)
	}

	fmt.Println("⏳ Generating the completion...")

	// Have a "chat" with Ollama 🦙
	// Prompt construction
	systemChatInstructions := `You are a useful AI agent. your job is to answer the user prompt.
	If you detect that the user prompt is related to a tool, ignore this part and focus on the other parts.
	`

	messages = []api.Message{
		{Role: "system", Content: systemChatInstructions},
		{Role: "user", Content: userInstructions},
		{Role: "user", Content: contentForThePrompt},
	}

	var TRUE = true
	reqChat := &api.ChatRequest{
		Model:    chatLLM,
		Messages: messages,
		Options: map[string]interface{}{
			"temperature":   0.0,
			"repeat_last_n": 2,
		},
		Stream: &TRUE,
	}

	answer := ""
	errChat := ollamaClient.Chat(ctx, reqChat, func(resp api.ChatResponse) error {
		answer += resp.Message.Content
		fmt.Print(resp.Message.Content)
		return nil
	})

	if errChat != nil {
		log.Fatalln("😡", err)
	}

}

// From: https://github.com/mark3labs/mcphost/blob/main/pkg/llm/ollama/provider.go
func ConvertToOllamaTools(tools []mcp.Tool) []api.Tool {
	// Convert tools to Ollama format
	ollamaTools := make([]api.Tool, len(tools))
	for i, tool := range tools {
		ollamaTools[i] = api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters: struct {
					Type       string   `json:"type"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        string   `json:"type"`
						Description string   `json:"description"`
						Enum        []string `json:"enum,omitempty"`
					} `json:"properties"`
				}{
					Type:       tool.InputSchema.Type,
					Required:   tool.InputSchema.Required,
					Properties: convertProperties(tool.InputSchema.Properties),
				},
			},
		}
	}
	return ollamaTools
}

// Helper function to convert properties to Ollama's format
func convertProperties(props map[string]interface{}) map[string]struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
} {
	result := make(map[string]struct {
		Type        string   `json:"type"`
		Description string   `json:"description"`
		Enum        []string `json:"enum,omitempty"`
	})

	for name, prop := range props {
		if propMap, ok := prop.(map[string]interface{}); ok {
			prop := struct {
				Type        string   `json:"type"`
				Description string   `json:"description"`
				Enum        []string `json:"enum,omitempty"`
			}{
				Type:        getString(propMap, "type"),
				Description: getString(propMap, "description"),
			}

			// Handle enum if present
			if enumRaw, ok := propMap["enum"].([]interface{}); ok {
				for _, e := range enumRaw {
					if str, ok := e.(string); ok {
						prop.Enum = append(prop.Enum, str)
					}
				}
			}

			result[name] = prop
		}
	}

	return result
}

// Helper function to safely get string values from map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
