package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"testing"

	"github.com/peterwwillis/hn-critique/internal/generator"
)

// mockProvider implements ai.Provider for unit tests without making real AI calls.
type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) AnalyzeArticle(title, articleURL, content string) (*generator.ArticleCritique, error) {
	return &generator.ArticleCritique{
		Summary:      "Test summary for " + title,
		MainPoints:   []string{"point1"},
		Truthfulness: "Test truthfulness",
		Rating:       "reliable",
	}, nil
}

func (m *mockProvider) AnalyzeComments(title, articleURL string, comments []*generator.Comment) (*generator.CommentsCritique, error) {
	analyzed := make([]generator.AnalyzedComment, len(comments))
	for i, c := range comments {
		analyzed[i] = generator.AnalyzedComment{
			ID:           c.ID,
			Author:       c.Author,
			Text:         string(c.Text),
			Indicators:   []string{"thoughtful"},
			AccuracyRank: i + 1,
			Analysis:     "Test analysis",
		}
	}
	return &generator.CommentsCritique{
		Summary:  "Test comments summary",
		Comments: analyzed,
	}, nil
}

// rawID returns a json.RawMessage containing the given integer as JSON.
func rawID(n int) *json.RawMessage {
	idBytes := json.RawMessage(fmt.Sprintf("%d", n))
	return &idBytes
}

func TestDispatch_Initialize(t *testing.T) {
	p := &mockProvider{}
	req := &rpcRequest{
		JSONRPC: "2.0",
		ID:      rawID(1),
		Method:  "initialize",
	}
	resp := dispatch(p, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	if result["protocolVersion"] != mcpProtocolVersion {
		t.Errorf("protocolVersion = %v, want %v", result["protocolVersion"], mcpProtocolVersion)
	}
}

func TestDispatch_ToolsList(t *testing.T) {
	p := &mockProvider{}
	req := &rpcRequest{
		JSONRPC: "2.0",
		ID:      rawID(2),
		Method:  "tools/list",
	}
	resp := dispatch(p, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	tools, ok := result["tools"].([]toolDef)
	if !ok {
		t.Fatalf("expected []toolDef, got %T", result["tools"])
	}
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names["analyze_article"] {
		t.Error("missing analyze_article tool")
	}
	if !names["analyze_comments"] {
		t.Error("missing analyze_comments tool")
	}
}

func TestDispatch_AnalyzeArticle(t *testing.T) {
	p := &mockProvider{}
	args, _ := json.Marshal(map[string]string{
		"title":   "Test Article",
		"url":     "https://example.com",
		"content": "Some content",
	})
	params, _ := json.Marshal(toolCallParams{
		Name:      "analyze_article",
		Arguments: args,
	})
	req := &rpcRequest{
		JSONRPC: "2.0",
		ID:      rawID(3),
		Method:  "tools/call",
		Params:  params,
	}
	resp := dispatch(p, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result, ok := resp.Result.(*toolResult)
	if !ok {
		t.Fatalf("expected *toolResult, got %T", resp.Result)
	}
	if result.IsError {
		t.Errorf("unexpected tool error: %s", result.Content[0].Text)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	var critique generator.ArticleCritique
	if err := json.Unmarshal([]byte(result.Content[0].Text), &critique); err != nil {
		t.Fatalf("failed to parse critique JSON: %v", err)
	}
	if critique.Rating != "reliable" {
		t.Errorf("Rating = %q, want %q", critique.Rating, "reliable")
	}
}

func TestDispatch_AnalyzeComments(t *testing.T) {
	p := &mockProvider{}
	args, _ := json.Marshal(map[string]any{
		"title": "Test Story",
		"url":   "https://example.com",
		"comments": []mcpComment{
			{ID: 1, Author: "alice", Text: "Great article!", Depth: 0},
			{ID: 2, Author: "bob", Text: "I disagree.", Depth: 0},
		},
	})
	params, _ := json.Marshal(toolCallParams{
		Name:      "analyze_comments",
		Arguments: args,
	})
	req := &rpcRequest{
		JSONRPC: "2.0",
		ID:      rawID(4),
		Method:  "tools/call",
		Params:  params,
	}
	resp := dispatch(p, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result, ok := resp.Result.(*toolResult)
	if !ok {
		t.Fatalf("expected *toolResult, got %T", resp.Result)
	}
	if result.IsError {
		t.Errorf("unexpected tool error: %s", result.Content[0].Text)
	}
	var cc generator.CommentsCritique
	if err := json.Unmarshal([]byte(result.Content[0].Text), &cc); err != nil {
		t.Fatalf("failed to parse CommentsCritique JSON: %v", err)
	}
	if len(cc.Comments) != 2 {
		t.Errorf("expected 2 analyzed comments, got %d", len(cc.Comments))
	}
}

func TestDispatch_UnknownMethod(t *testing.T) {
	p := &mockProvider{}
	req := &rpcRequest{
		JSONRPC: "2.0",
		ID:      rawID(5),
		Method:  "no/such/method",
	}
	resp := dispatch(p, req)
	if resp.Error == nil {
		t.Fatal("expected an error response for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
}

func TestDispatch_UnknownTool(t *testing.T) {
	p := &mockProvider{}
	params, _ := json.Marshal(toolCallParams{
		Name:      "no_such_tool",
		Arguments: json.RawMessage(`{}`),
	})
	req := &rpcRequest{
		JSONRPC: "2.0",
		ID:      rawID(6),
		Method:  "tools/call",
		Params:  params,
	}
	resp := dispatch(p, req)
	// Unknown tool returns a tool-level error (isError:true), not a JSON-RPC error.
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}
	result, ok := resp.Result.(*toolResult)
	if !ok {
		t.Fatalf("expected *toolResult, got %T", resp.Result)
	}
	if !result.IsError {
		t.Error("expected isError=true for unknown tool")
	}
}

func TestToGeneratorComments(t *testing.T) {
	src := []mcpComment{
		{
			ID: 1, Author: "alice", Text: "<p>Hello</p>", Time: 1234567890, Depth: 0,
			Kids: []mcpComment{
				{ID: 2, Author: "bob", Text: "Reply", Depth: 1},
			},
		},
	}
	got := toGeneratorComments(src)
	if len(got) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(got))
	}
	if got[0].ID != 1 {
		t.Errorf("ID = %d, want 1", got[0].ID)
	}
	if got[0].Text != template.HTML("<p>Hello</p>") {
		t.Errorf("Text = %q, want %q", got[0].Text, "<p>Hello</p>")
	}
	if got[0].Time != 1234567890 {
		t.Errorf("Time = %d, want 1234567890", got[0].Time)
	}
	if len(got[0].Kids) != 1 {
		t.Fatalf("expected 1 child comment, got %d", len(got[0].Kids))
	}
	if got[0].Kids[0].Author != "bob" {
		t.Errorf("child author = %q, want %q", got[0].Kids[0].Author, "bob")
	}
}
