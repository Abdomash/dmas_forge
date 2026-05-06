package workflow

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"

	openai "github.com/openai/openai-go"
	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
)

//go:embed all:data/support
var supportDocs embed.FS

const supportPrompt = `You are SupportAgent for hotel booking and policy support. Use automatic policy context when available. You may call lookup_booking only when the user provides a booking ID. Do not search hotels, check availability, create, change, or cancel bookings.`

type SupportAgentImpl struct {
	agent       core.Agent
	kb          core.KnowledgeBase
	reservation ReservationService
}

func NewSupportAgentImpl(ctx context.Context, agent core.Agent, kb core.KnowledgeBase, reservation ReservationService) (SupportAgent, error) {
	s := &SupportAgentImpl{
		agent:       agent,
		kb:          kb,
		reservation: reservation,
	}

	if err := agent.AddSystemPrompt(ctx, supportPrompt); err != nil {
		return nil, err
	}

	if err := agent.AddTools(ctx, map[string]openai.ChatCompletionToolParam{"lookup_booking": lookupBookingTool()}); err != nil {
		return nil, err
	}

	if err := agent.RegisterToolCallHandler(ctx, s.handleTool); err != nil {
		return nil, err
	}

	if err := indexSupportDocs(ctx, kb); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *SupportAgentImpl) AskSupport(ctx context.Context, req SupportRequest) (SupportResult, error) {
	ans, err := s.agent.LLMCallWithTools(ctx, req.Question)
	if err != nil {
		return SupportResult{}, err
	}

	return SupportResult{Answer: ans}, nil
}

func lookupBookingTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Function: openai.FunctionDefinitionParam{
			Name:        "lookup_booking",
			Description: openai.String("Look up an existing booking by booking ID."),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"booking_id": map[string]interface{}{"type": "string"},
				},
				"required": []string{"booking_id"},
			},
		},
	}
}

func (s *SupportAgentImpl) handleTool(ctx context.Context, tc openai.ChatCompletionMessageToolCall) (string, error) {
	if tc.Function.Name != "lookup_booking" {
		return "", fmt.Errorf("unsupported tool: %s", tc.Function.Name)
	}

	var args struct {
		BookingID string `json:"booking_id"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return "", err
	}

	booking, err := s.reservation.GetBooking(ctx, args.BookingID)
	if err != nil {
		return "", err
	}

	b, err := json.Marshal(booking)
	return string(b), err
}

func indexSupportDocs(ctx context.Context, kb core.KnowledgeBase) error {
	entries, err := fs.ReadDir(supportDocs, "data/support")
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}

		path := filepath.Join("data/support", e.Name())
		content, err := fs.ReadFile(supportDocs, path)
		if err != nil {
			return err
		}

		id := e.Name()[:len(e.Name())-len(filepath.Ext(e.Name()))]
		doc := core.Document{
			ID:       id,
			Content:  string(content),
			Metadata: map[string]interface{}{"file_name": e.Name()},
		}
		if err = kb.Index(ctx, doc); err != nil {
			return err
		}
	}

	return nil
}
