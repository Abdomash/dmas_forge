package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	openai "github.com/openai/openai-go"
	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
)

const advisorPrompt = `You are HotelAdvisorAgent, a pre-booking hotel discovery assistant. Use only read-only tools for hotel search, rates, profiles, and availability. Do not create bookings or look up bookings. If required details for a tool call are missing, ask a concise clarification instead of guessing.`

type HotelAdvisorAgentImpl struct {
	agent       core.Agent
	search      SearchService
	rate        RateService
	profile     ProfileService
	reservation ReservationService
}

func NewHotelAdvisorAgentImpl(ctx context.Context, agent core.Agent, search SearchService, rate RateService, profile ProfileService, reservation ReservationService) (HotelAdvisorAgent, error) {
	a := &HotelAdvisorAgentImpl{
		agent:       agent,
		search:      search,
		rate:        rate,
		profile:     profile,
		reservation: reservation,
	}

	if err := agent.AddSystemPrompt(ctx, advisorPrompt); err != nil {
		return nil, err
	}

	if err := agent.AddTools(ctx, advisorTools()); err != nil {
		return nil, err
	}

	if err := agent.RegisterToolCallHandler(ctx, a.handleTool); err != nil {
		return nil, err
	}

	return a, nil
}

func (a *HotelAdvisorAgentImpl) PlanStay(ctx context.Context, req AdvisorRequest) (AdvisorResult, error) {
	ans, err := a.agent.LLMCallWithTools(ctx, req.Prompt)
	if err != nil {
		return AdvisorResult{}, err
	}

	return AdvisorResult{Answer: ans}, nil
}

func advisorTools() map[string]openai.ChatCompletionToolParam {
	tool := func(name, desc string, props map[string]interface{}, required []string) openai.ChatCompletionToolParam {
		return openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        name,
				Description: openai.String(desc),
				Parameters: openai.FunctionParameters{
					"type":       "object",
					"properties": props,
					"required":   required,
				},
			},
		}
	}

	return map[string]openai.ChatCompletionToolParam{
		"search_hotels": tool(
			"search_hotels",
			"Find candidate hotel IDs near coordinates.",
			map[string]interface{}{
				"lat": map[string]interface{}{"type": "number"},
				"lon": map[string]interface{}{"type": "number"},
			},
			[]string{"lat", "lon"},
		),
		"get_rates": tool(
			"get_rates",
			"Get rate plans for hotel IDs.",
			map[string]interface{}{
				"hotel_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
			},
			[]string{"hotel_ids"},
		),
		"get_profiles": tool(
			"get_profiles",
			"Get hotel profiles.",
			map[string]interface{}{
				"hotel_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"locale": map[string]interface{}{"type": "string"},
			},
			[]string{"hotel_ids", "locale"},
		),
		"check_availability": tool(
			"check_availability",
			"Check available hotels for a stay.",
			map[string]interface{}{
				"customer_name": map[string]interface{}{"type": "string"},
				"hotel_ids": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"in_date":    map[string]interface{}{"type": "string"},
				"out_date":   map[string]interface{}{"type": "string"},
				"room_count": map[string]interface{}{"type": "integer"},
			},
			[]string{"hotel_ids", "in_date", "out_date", "room_count"},
		),
	}
}

func (a *HotelAdvisorAgentImpl) handleTool(ctx context.Context, tc openai.ChatCompletionMessageToolCall) (string, error) {
	switch tc.Function.Name {
	case "search_hotels":
		var x struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &x); err != nil {
			return "", err
		}

		return marshalToolResult(a.search.Nearby(ctx, x.Lat, x.Lon))
	case "get_rates":
		var x struct {
			HotelIDs []string `json:"hotel_ids"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &x); err != nil {
			return "", err
		}

		return marshalToolResult(a.rate.GetRates(ctx, x.HotelIDs))
	case "get_profiles":
		var x struct {
			HotelIDs []string `json:"hotel_ids"`
			Locale   string   `json:"locale"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &x); err != nil {
			return "", err
		}

		return marshalToolResult(a.profile.GetProfiles(ctx, x.HotelIDs, x.Locale))
	case "check_availability":
		var x struct {
			CustomerName string   `json:"customer_name"`
			HotelIDs     []string `json:"hotel_ids"`
			InDate       string   `json:"in_date"`
			OutDate      string   `json:"out_date"`
			RoomCount    int64    `json:"room_count"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &x); err != nil {
			return "", err
		}

		return marshalToolResult(a.reservation.CheckAvailability(ctx, x.CustomerName, x.HotelIDs, x.InDate, x.OutDate, x.RoomCount))
	default:
		return "", fmt.Errorf("unsupported tool: %s", tc.Function.Name)
	}
}

func marshalToolResult(v interface{}, err error) (string, error) {
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(v)
	return string(out), err
}
