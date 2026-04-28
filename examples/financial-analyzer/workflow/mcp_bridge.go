package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	openai "github.com/openai/openai-go"
	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const mcpBridgeTracerName = "github.com/vaastav/agentic_blueprint/examples/financial-analyzer/workflow/mcp_bridge"

type MCPToolBridge struct {
	sessions  []*mcp.ClientSession
	toolToIdx map[string]int
	tools     map[string]openai.ChatCompletionToolParam
}

func NewMCPToolBridge(ctx context.Context, serverURLs []string) (*MCPToolBridge, error) {
	b := &MCPToolBridge{
		toolToIdx: make(map[string]int),
		tools:     make(map[string]openai.ChatCompletionToolParam),
	}

	if len(serverURLs) == 0 {
		return nil, fmt.Errorf("at least one MCP server URL is required")
	}

	for _, rawURL := range serverURLs {
		url := strings.TrimSpace(rawURL)
		if url == "" {
			continue
		}
		tracer := trace.SpanFromContext(ctx).TracerProvider().Tracer(mcpBridgeTracerName)
		serverCtx, span := tracer.Start(ctx, "mcp.discovery",
			trace.WithAttributes(
				attribute.String("mcp.server_url", url),
				attribute.String("provider_mode", "external"),
			),
		)

		client := mcp.NewClient(&mcp.Implementation{
			Name:    "financial-analyzer-mcp-bridge",
			Version: "1.0.0",
		}, nil)

		session, err := client.Connect(serverCtx, &mcp.StreamableClientTransport{
			Endpoint: url,
		}, nil)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			log.Printf("MCP bridge: failed to connect to server %s: %v", url, err)
			continue
		}

		result, err := session.ListTools(serverCtx, nil)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			log.Printf("MCP bridge: failed to list tools from %s: %v", url, err)
			_ = session.Close()
			continue
		}
		span.SetAttributes(attribute.Int("mcp.tool_count", len(result.Tools)))
		span.SetStatus(codes.Ok, "")
		span.End()

		idx := len(b.sessions)
		b.sessions = append(b.sessions, session)

		for _, tool := range result.Tools {
			if _, exists := b.toolToIdx[tool.Name]; exists {
				log.Printf("MCP bridge: tool %q already registered from another server; overriding with definition from %s", tool.Name, url)
			}
			openaiTool, err := mcpToolToOpenAI(tool)
			if err != nil {
				log.Printf("MCP bridge: skipping tool %q from %s: %v", tool.Name, url, err)
				continue
			}
			b.toolToIdx[tool.Name] = idx
			b.tools[tool.Name] = openaiTool
		}
	}

	if len(b.tools) == 0 {
		for _, session := range b.sessions {
			_ = session.Close()
		}
		return nil, fmt.Errorf("no tools discovered from any MCP server")
	}

	log.Printf("MCP bridge: connected to %d server(s), discovered %d tool(s)", len(b.sessions), len(b.tools))
	return b, nil
}

func (b *MCPToolBridge) AddToolsToAgent(ctx context.Context, agent core.Agent) error {
	return agent.AddTools(ctx, b.tools)
}

func (b *MCPToolBridge) HandleToolCall(ctx context.Context, tc openai.ChatCompletionMessageToolCall) (string, error) {
	tracer := trace.SpanFromContext(ctx).TracerProvider().Tracer(mcpBridgeTracerName)
	ctx, span := tracer.Start(ctx, "mcp.tool_call",
		trace.WithAttributes(
			attribute.String("tool.name", tc.Function.Name),
			attribute.String("provider_mode", "external"),
		),
	)
	defer span.End()

	idx, ok := b.toolToIdx[tc.Function.Name]
	if !ok {
		span.SetStatus(codes.Error, "tool not found")
		return fmt.Sprintf("Error: tool %q not found in MCP bridge", tc.Function.Name), nil
	}
	span.SetAttributes(attribute.Int("mcp.session_index", idx))

	var args map[string]any
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return "", fmt.Errorf("parsing tool call arguments for %q: %w", tc.Function.Name, err)
		}
	}
	if args == nil {
		args = make(map[string]any)
	}

	result, err := b.sessions[idx].CallTool(ctx, &mcp.CallToolParams{
		Name:      tc.Function.Name,
		Arguments: args,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Sprintf("Error calling MCP tool %q: %v", tc.Function.Name, err), nil
	}

	var parts []string
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		} else {
			parts = append(parts, fmt.Sprintf("%v", content))
		}
	}

	output := strings.Join(parts, "\n")
	if result.IsError && output != "" {
		output = "Error from MCP server: " + output
		span.SetStatus(codes.Error, "mcp tool returned error")
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.SetAttributes(attribute.Bool("mcp.tool_is_error", result.IsError))
	return output, nil
}

func (b *MCPToolBridge) Close() error {
	var firstErr error
	for _, session := range b.sessions {
		if err := session.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func mcpToolToOpenAI(tool *mcp.Tool) (openai.ChatCompletionToolParam, error) {
	params := openai.FunctionParameters{
		"type": "object",
	}
	if tool.InputSchema != nil {
		switch schema := tool.InputSchema.(type) {
		case map[string]any:
			for k, v := range schema {
				params[k] = v
			}
		default:
			encoded, err := json.Marshal(tool.InputSchema)
			if err != nil {
				return openai.ChatCompletionToolParam{}, fmt.Errorf("marshaling input schema for tool %q: %w", tool.Name, err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				return openai.ChatCompletionToolParam{}, fmt.Errorf("unmarshaling input schema for tool %q: %w", tool.Name, err)
			}
			for k, v := range decoded {
				params[k] = v
			}
		}
	}
	if _, ok := params["type"]; !ok {
		params["type"] = "object"
	}

	return openai.ChatCompletionToolParam{
		Function: openai.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: openai.String(tool.Description),
			Parameters:  params,
		},
	}, nil
}
