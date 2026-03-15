// Command mcp-server implements the Model Context Protocol (MCP) over stdio,
// exposing AI analysis functions as tools that any MCP-compatible client can call.
//
// The server reads newline-delimited JSON-RPC 2.0 messages from stdin and
// writes responses to stdout. All diagnostic logging goes to stderr.
//
// # Exposed tools
//
//   - analyze_article  — factual critique of a news article
//   - analyze_comments — ranked critique of a Hacker News comment section
//
// # Configuration
//
// Configuration follows the same rules as the crawler: environment variables
// (GITHUB_TOKEN, OPENAI_API_KEY, …) and an optional hn-critique.toml in the
// working directory.  The AI provider can be overridden with the -provider flag.
//
// # Usage
//
//	./bin/mcp-server
//	./bin/mcp-server -provider openai
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"

	"github.com/peterwwillis/hn-critique/internal/ai"
	"github.com/peterwwillis/hn-critique/internal/config"
	"github.com/peterwwillis/hn-critique/internal/generator"
)

const mcpProtocolVersion = "2024-11-05"

// maxMessageSize is the maximum size in bytes of a single MCP message.
// 4 MiB accommodates large article content payloads.
const maxMessageSize = 4 * 1024 * 1024

// ── JSON-RPC 2.0 wire types ─────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ── MCP tool definitions ─────────────────────────────────────────────────────

type toolDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema toolSchema `json:"inputSchema"`
}

type toolSchema struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
	Required   []string       `json:"required,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// mcpComment is the wire format used when comments are supplied to a tool call.
type mcpComment struct {
	ID     int          `json:"id"`
	Author string       `json:"author"`
	Text   string       `json:"text"`
	Time   int64        `json:"time,omitempty"`
	Depth  int          `json:"depth"`
	Kids   []mcpComment `json:"kids,omitempty"`
}

var mcpTools = []toolDef{
	{
		Name: "analyze_article",
		Description: "Analyze a news article for factual accuracy and generate a structured " +
			"critique that includes a summary, main points, truthfulness assessment, " +
			"considerations, and a rating (reliable, needs citation, questionable, or misleading).",
		InputSchema: toolSchema{
			Type: "object",
			Properties: map[string]any{
				"title":   map[string]string{"type": "string", "description": "Article title"},
				"url":     map[string]string{"type": "string", "description": "Article URL"},
				"content": map[string]string{"type": "string", "description": "Full text of the article to analyze"},
			},
			Required: []string{"title", "url"},
		},
	},
	{
		Name: "analyze_comments",
		Description: "Analyze a set of Hacker News comments and generate a ranked critique " +
			"that identifies insightful, misleading, emotional, or low-quality contributions.",
		InputSchema: toolSchema{
			Type: "object",
			Properties: map[string]any{
				"title": map[string]string{"type": "string", "description": "Story title"},
				"url":   map[string]string{"type": "string", "description": "Story URL"},
				"comments": map[string]any{
					"type":        "array",
					"description": "Top-level comments to analyze",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":     map[string]string{"type": "integer", "description": "Comment ID"},
							"author": map[string]string{"type": "string", "description": "Username"},
							"text":   map[string]string{"type": "string", "description": "Comment text (may contain HTML)"},
							"time":   map[string]string{"type": "integer", "description": "Unix timestamp"},
							"depth":  map[string]string{"type": "integer", "description": "Nesting depth (0 = top-level)"},
							"kids": map[string]any{
								"type":        "array",
								"description": "Nested child comments",
								"items":       map[string]string{"type": "object"},
							},
						},
					},
				},
			},
			Required: []string{"title", "url", "comments"},
		},
	},
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	// Diagnostic output always goes to stderr; stdout is the MCP protocol channel.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags)

	providerFlag := flag.String("provider", "", "AI provider to use: openai, github (overrides config file)")
	configPath := flag.String("config", "", "path to TOML config file (default: hn-critique.toml if present)")
	flag.Parse()

	// Auto-detect config file.
	if *configPath == "" {
		if _, err := os.Stat("hn-critique.toml"); err == nil {
			*configPath = "hn-critique.toml"
		}
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("mcp-server: config error: %v", err)
	}
	if *providerFlag != "" {
		cfg.Provider = config.ProviderName(*providerFlag)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("mcp-server: config validation: %v", err)
	}

	provider, err := ai.NewProvider(cfg)
	if err != nil {
		log.Fatalf("mcp-server: provider error: %v", err)
	}
	log.Printf("mcp-server: started with provider %s", provider.Name())

	run(os.Stdin, os.Stdout, provider)
}

// run is the main message loop, separated for testability.
func run(in *os.File, out *os.File, provider ai.Provider) {
	enc := json.NewEncoder(out)
	scanner := bufio.NewScanner(in)
	// Allow up to maxMessageSize per message (large article content).
	scanner.Buffer(make([]byte, maxMessageSize), maxMessageSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("mcp-server: parse error: %v", err)
			continue
		}

		// MCP notifications have no id and require no response.
		if req.ID == nil {
			log.Printf("mcp-server: notification %s", req.Method)
			continue
		}

		resp := dispatch(provider, &req)
		if err := enc.Encode(resp); err != nil {
			log.Printf("mcp-server: encode error: %v", err)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("mcp-server: stdin error: %v", err)
	}
}

// ── Request dispatcher ────────────────────────────────────────────────────────

func dispatch(provider ai.Provider, req *rpcRequest) *rpcResponse {
	switch req.Method {
	case "initialize":
		return okResp(req.ID, map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "hn-critique", "version": "1.0.0"},
		})

	case "tools/list":
		return okResp(req.ID, map[string]any{"tools": mcpTools})

	case "tools/call":
		var p toolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errResp(req.ID, -32602, "invalid params: "+err.Error())
		}
		result, err := callTool(provider, p.Name, p.Arguments)
		if err != nil {
			// Surface tool-level errors inside the result payload (not as JSON-RPC errors)
			// so that MCP clients can display them without treating them as protocol failures.
			return okResp(req.ID, &toolResult{
				Content: []toolContent{{Type: "text", Text: err.Error()}},
				IsError: true,
			})
		}
		return okResp(req.ID, result)

	default:
		return errResp(req.ID, -32601, "method not found: "+req.Method)
	}
}

// ── Tool implementations ──────────────────────────────────────────────────────

func callTool(provider ai.Provider, name string, args json.RawMessage) (*toolResult, error) {
	switch name {
	case "analyze_article":
		return callAnalyzeArticle(provider, args)
	case "analyze_comments":
		return callAnalyzeComments(provider, args)
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func callAnalyzeArticle(provider ai.Provider, args json.RawMessage) (*toolResult, error) {
	var a struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	critique, err := provider.AnalyzeArticle(a.Title, a.URL, a.Content)
	if err != nil {
		return nil, err
	}
	text, err := json.Marshal(critique)
	if err != nil {
		return nil, err
	}
	return &toolResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}

func callAnalyzeComments(provider ai.Provider, args json.RawMessage) (*toolResult, error) {
	var a struct {
		Title    string       `json:"title"`
		URL      string       `json:"url"`
		Comments []mcpComment `json:"comments"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	comments := toGeneratorComments(a.Comments)
	critique, err := provider.AnalyzeComments(a.Title, a.URL, comments)
	if err != nil {
		return nil, err
	}
	text, err := json.Marshal(critique)
	if err != nil {
		return nil, err
	}
	return &toolResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}

// toGeneratorComments converts the MCP wire format into internal Comment types.
func toGeneratorComments(src []mcpComment) []*generator.Comment {
	dst := make([]*generator.Comment, 0, len(src))
	for _, c := range src {
		gc := &generator.Comment{
			ID:     c.ID,
			Author: c.Author,
			Text:   template.HTML(c.Text), //nolint:gosec // text is provided by the caller, not from HN
			Time:   c.Time,
			Depth:  c.Depth,
		}
		if len(c.Kids) > 0 {
			gc.Kids = toGeneratorComments(c.Kids)
		}
		dst = append(dst, gc)
	}
	return dst
}

// ── JSON-RPC helpers ──────────────────────────────────────────────────────────

func okResp(id *json.RawMessage, result any) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errResp(id *json.RawMessage, code int, msg string) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}
