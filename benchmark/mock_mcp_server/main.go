package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type companyProfile struct {
	Name      string
	Ticker    string
	Price     string
	MarketCap string
	Revenue   string
	NetIncome string
	Source    string
	Date      string
	Summary   string
}

type searchParams struct {
	Query string `json:"query"`
}

type fetchParams struct {
	URL string `json:"url"`
}

var profiles = map[string]companyProfile{
	"apple": {
		Name:      "Apple",
		Ticker:    "AAPL",
		Price:     "$212.47",
		MarketCap: "$3.2T",
		Revenue:   "$391.0B trailing twelve months",
		NetIncome: "$96.9B trailing twelve months",
		Source:    "Benchmark Mock Finance Feed",
		Date:      "2026-04-01",
		Summary:   "Apple remains a large-cap consumer hardware and services company with durable cash generation and a high-margin services mix.",
	},
	"microsoft": {
		Name:      "Microsoft",
		Ticker:    "MSFT",
		Price:     "$468.12",
		MarketCap: "$3.5T",
		Revenue:   "$261.8B trailing twelve months",
		NetIncome: "$96.5B trailing twelve months",
		Source:    "Benchmark Mock Finance Feed",
		Date:      "2026-04-01",
		Summary:   "Microsoft combines large enterprise software cash flows with cloud growth from Azure and related AI platform demand.",
	},
	"nvidia": {
		Name:      "NVIDIA",
		Ticker:    "NVDA",
		Price:     "$128.44",
		MarketCap: "$3.1T",
		Revenue:   "$130.5B trailing twelve months",
		NetIncome: "$72.9B trailing twelve months",
		Source:    "Benchmark Mock Finance Feed",
		Date:      "2026-04-01",
		Summary:   "NVIDIA is driven by accelerated computing demand, with data-center GPUs remaining the core growth engine.",
	},
}

func main() {
	listen := flag.String("listen", "localhost:8080", "address for the mock MCP server")
	flag.Parse()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "benchmark-mock-mcp-server",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_web",
		Description: "Return deterministic mock financial search results for a company query.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type": "string",
				},
			},
			"required": []string{"query"},
		},
	}, searchWeb)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fetch_url",
		Description: "Return deterministic mock page content for a URL returned by search_web.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type": "string",
				},
			},
			"required": []string{"url"},
		},
	}, fetchURL)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, nil)

	log.Printf("mock MCP server listening on http://%s", *listen)
	log.Fatal(http.ListenAndServe(*listen, handler))
}

func searchWeb(ctx context.Context, req *mcp.CallToolRequest, params *searchParams) (*mcp.CallToolResult, any, error) {
	_ = ctx
	_ = req
	profile := matchProfile(params.Query)
	content := fmt.Sprintf(
		"1. %s overview (%s)\nURL: https://benchmark.mock/%s/overview\nSummary: %s\n\n2. %s financials (%s)\nURL: https://benchmark.mock/%s/financials\nSummary: Price %s, market cap %s, revenue %s, net income %s.\n\n3. %s outlook (%s)\nURL: https://benchmark.mock/%s/outlook\nSummary: Current operating context and investment watchpoints for %s.",
		profile.Name,
		profile.Source,
		profileSlug(profile),
		profile.Summary,
		profile.Name,
		profile.Source,
		profileSlug(profile),
		profile.Price,
		profile.MarketCap,
		profile.Revenue,
		profile.NetIncome,
		profile.Name,
		profile.Source,
		profileSlug(profile),
		profile.Name,
	)
	return textResult(content), nil, nil
}

func fetchURL(ctx context.Context, req *mcp.CallToolRequest, params *fetchParams) (*mcp.CallToolResult, any, error) {
	_ = ctx
	_ = req
	profile := matchProfile(params.URL)
	page := strings.Trim(strings.TrimSpace(params.URL), "/")
	content := fmt.Sprintf(
		"# %s mock source\n\n- Source: %s\n- Date: %s\n- Ticker: %s\n- Current price: %s\n- Market cap: %s\n- Revenue: %s\n- Net income: %s\n\n## Summary\n%s\n\n## Notes\nThis is deterministic benchmark fixture data served by benchmark/mock_mcp_server. Page key: %s.",
		profile.Name,
		profile.Source,
		profile.Date,
		profile.Ticker,
		profile.Price,
		profile.MarketCap,
		profile.Revenue,
		profile.NetIncome,
		profile.Summary,
		page,
	)
	return textResult(content), nil, nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func matchProfile(value string) companyProfile {
	lower := strings.ToLower(value)
	for key, profile := range profiles {
		if strings.Contains(lower, key) || strings.Contains(lower, strings.ToLower(profile.Ticker)) {
			return profile
		}
	}
	return companyProfile{
		Name:      "Example Company",
		Ticker:    "EXM",
		Price:     "$100.00",
		MarketCap: "$100B",
		Revenue:   "$10B trailing twelve months",
		NetIncome: "$2B trailing twelve months",
		Source:    "Benchmark Mock Finance Feed",
		Date:      "2026-04-01",
		Summary:   "Fallback mock profile used when the query does not match a benchmark fixture company.",
	}
}

func profileSlug(profile companyProfile) string {
	return strings.ToLower(strings.ReplaceAll(profile.Name, " ", "-"))
}
