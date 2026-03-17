package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	openai "github.com/openai/openai-go"
	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
)

const ragSystemPromptSuffix = "\n\nYou have access to a retrieval-augmented generation (RAG) capabilities. " +
	"Use `search_knowledge` tool to search your knowledge base for relevant information. " +
	"Use `index_document` tool to add new documents to your knowledge base. " +
	"Use `delete_document` tool to remove documents from your knowledge base when they are no longer relevant or accurate. " +
	"Proactively use your knowledge base when it would improve your responses."

var ragToolDefs = map[string]openai.ChatCompletionToolParam{
	"search_knowledge": {
		Function: openai.FunctionDefinitionParam{
			Name: "search_knowledge",
			Description: openai.String(
				"Search your knowledge base for relevant information. " +
					"Returns chunks of text ranked by relevance score."),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]string{
						"type":        "string",
						"description": "The search query (keywords or natural language)",
					},
					"top_k": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results to return (default 5)",
					},
				},
				"required": []string{"query"},
			},
		},
	},
	"index_document": {
		Function: openai.FunctionDefinitionParam{
			Name: "index_document",
			Description: openai.String(
				"Add a new document to your knowledge base. " +
					"Use clear, descriptive keys (e.g., 'product_manual', 'company_policy'). " +
					"Values should be concise but complete."),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]string{
						"type":        "string",
						"description": "The full text content of the document",
					},
					"title": map[string]string{
						"type":        "string",
						"description": "A short title for the document (optional)",
					},
					"source": map[string]string{
						"type":        "string",
						"description": "Original source of the document (for reference)",
					},
					"doc_id": map[string]string{
						"type":        "string",
						"description": "Unique identifier for this document",
					},
				},
				"required": []string{"content"},
			},
		},
	},
	"delete_document": {
		Function: openai.FunctionDefinitionParam{
			Name: "delete_document",
			Description: openai.String(
				"Remove a document from your knowledge base when it is no longer relevant or accurate."),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"doc_id": map[string]string{
						"type":        "string",
						"description": "The exact document ID to remove",
					},
				},
				"required": []string{"doc_id"},
			},
		},
	},
}

var ragToolNames = map[string]bool{
	"search_knowledge": true,
	"index_document":   true,
	"delete_document":  true,
}

type RAGAgentConfig struct {
	ReadOnly bool
}

type RAGAgent struct {
	inner       core.Agent
	kb          core.KnowledgeBase
	userHandler core.ToolHandlerFn
	config      RAGAgentConfig
}

func NewRAGAgent(ctx context.Context, agent core.Agent, kb core.KnowledgeBase, config RAGAgentConfig) (*RAGAgent, error) {
	r := &RAGAgent{
		inner:  agent,
		kb:     kb,
		config: config,
	}

	toolsToRegister := map[string]openai.ChatCompletionToolParam{
		"search_knowledge": ragToolDefs["search_knowledge"],
	}
	if !config.ReadOnly {
		toolsToRegister["index_document"] = ragToolDefs["index_document"]
		toolsToRegister["delete_document"] = ragToolDefs["delete_document"]
	}

	err := agent.AddTools(ctx, toolsToRegister)
	if err != nil {
		return nil, fmt.Errorf("rag agent: failed to add rag tools: %w", err)
	}

	err = agent.RegisterToolCallHandler(ctx, r.buildCompositeHandler())
	if err != nil {
		return nil, fmt.Errorf("rag agent: failed to register tool handler: %w", err)
	}

	return r, nil
}

func (r *RAGAgent) AddSystemPrompt(ctx context.Context, prompt string) error {
	return r.inner.AddSystemPrompt(ctx, prompt+ragSystemPromptSuffix)
}

func (r *RAGAgent) AddTools(ctx context.Context, tooldefs map[string]openai.ChatCompletionToolParam) error {
	return r.inner.AddTools(ctx, tooldefs)
}

func (r *RAGAgent) LLMCall(ctx context.Context, query string) (string, error) {
	return r.inner.LLMCall(ctx, query)
}

func (r *RAGAgent) LLMCallWithTools(ctx context.Context, query string) (string, error) {
	return r.inner.LLMCallWithTools(ctx, query)
}

func (r *RAGAgent) RegisterToolCallHandler(ctx context.Context, toolHandlerFn core.ToolHandlerFn) error {
	r.userHandler = toolHandlerFn
	return r.inner.RegisterToolCallHandler(ctx, r.buildCompositeHandler())
}

func (r *RAGAgent) buildCompositeHandler() core.ToolHandlerFn {
	return func(ctx context.Context, tc openai.ChatCompletionMessageToolCall) (string, error) {
		if ragToolNames[tc.Function.Name] {
			return r.handleRAGToolCall(ctx, tc)
		}
		if r.userHandler != nil {
			return r.userHandler(ctx, tc)
		}
		return "", fmt.Errorf("unsupported tool call: %s", tc.Function.Name)
	}
}

func (r *RAGAgent) handleRAGToolCall(ctx context.Context, tc openai.ChatCompletionMessageToolCall) (string, error) {
	switch tc.Function.Name {
	case "search_knowledge":
		return r.handleSearchKnowledge(ctx, tc)
	case "index_document":
		return r.handleIndexDocument(ctx, tc)
	case "delete_document":
		return r.handleDeleteDocument(ctx, tc)
	default:
		return "", fmt.Errorf("unknown rag tool: %s", tc.Function.Name)
	}
}

func (r *RAGAgent) handleSearchKnowledge(ctx context.Context, tc openai.ChatCompletionMessageToolCall) (string, error) {
	var args struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("search_knowledge: invalid arguments: %w", err)
	}

	if args.TopK <= 0 {
		args.TopK = 5
	}

	chunks, err := r.kb.Query(ctx, args.Query, args.TopK)
	if err != nil {
		return "", fmt.Errorf("search_knowledge: %w", err)
	}

	if len(chunks) == 0 {
		return "No relevant information found.", nil
	}

	var results []string
	for i, chunk := range chunks {
		results = append(results,
			fmt.Sprintf("[%d] Source: %s (Score: %.2f)\n%s", i+1, chunk.SourceDocID, chunk.Score, chunk.Content),
		)
	}

	return "Found " + strconv.Itoa(len(chunks)) + " relevant chunks:\n\n" + strings.Join(results, "\n\n"), nil
}

func (r *RAGAgent) handleIndexDocument(ctx context.Context, tc openai.ChatCompletionMessageToolCall) (string, error) {
	if r.config.ReadOnly {
		return "Indexing documents is disabled in read-only mode.", nil
	}

	var args struct {
		Content string `json:"content"`
		Title   string `json:"title"`
		Source  string `json:"source"`
		DocID   string `json:"doc_id"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("index_document: invalid arguments: %w", err)
	}

	docID := args.DocID
	if docID == "" {
		docID = args.Title
	}
	if docID == "" {
		docID = "doc_" + strconv.Itoa(int(ctx.Value("timestamp").(int64)))
	}

	doc := core.Document{
		ID:      docID,
		Content: args.Content,
		Metadata: map[string]any{
			"title":  args.Title,
			"source": args.Source,
		},
	}

	if err := r.kb.Index(ctx, doc); err != nil {
		return "", fmt.Errorf("index_document: %w", err)
	}

	return fmt.Sprintf("Document '%s' indexed successfully.", docID), nil
}

func (r *RAGAgent) handleDeleteDocument(ctx context.Context, tc openai.ChatCompletionMessageToolCall) (string, error) {
	if r.config.ReadOnly {
		return "Deleting documents is disabled in read-only mode.", nil
	}

	var args struct {
		DocID string `json:"doc_id"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("delete_document: invalid arguments: %w", err)
	}

	if err := r.kb.Delete(ctx, args.DocID); err != nil {
		return "", fmt.Errorf("delete_document: %w", err)
	}

	return fmt.Sprintf("Document '%s' deleted successfully.", args.DocID), nil
}
