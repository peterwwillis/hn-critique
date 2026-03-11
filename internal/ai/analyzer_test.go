package ai_test

import (
	"testing"

	"github.com/peterwwillis/hn-critique/internal/ai"
	"github.com/peterwwillis/hn-critique/internal/generator"
)

func TestNewAnalyzer(t *testing.T) {
	a := ai.NewAnalyzer("test-key")
	if a == nil {
		t.Error("NewAnalyzer returned nil")
	}
}

func TestParseJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(*generator.ArticleCritique) bool
	}{
		{
			name: "plain JSON",
			input: `{
				"summary": "A test summary",
				"mainPoints": ["point1", "point2"],
				"truthfulness": "Seems accurate",
				"considerations": ["note1"],
				"rating": "reliable"
			}`,
			wantErr: false,
			check: func(c *generator.ArticleCritique) bool {
				return c.Summary == "A test summary" && c.Rating == "reliable" && len(c.MainPoints) == 2
			},
		},
		{
			name: "JSON wrapped in markdown code fence",
			input: "```json\n{\"summary\":\"wrapped\",\"mainPoints\":[],\"truthfulness\":\"ok\",\"considerations\":[],\"rating\":\"questionable\"}\n```",
			wantErr: false,
			check: func(c *generator.ArticleCritique) bool {
				return c.Summary == "wrapped" && c.Rating == "questionable"
			},
		},
		{
			name: "JSON with preamble text",
			input: "Here is the analysis:\n{\"summary\":\"preamble test\",\"mainPoints\":[],\"truthfulness\":\"ok\",\"considerations\":[],\"rating\":\"reliable\"}",
			wantErr: false,
			check: func(c *generator.ArticleCritique) bool {
				return c.Summary == "preamble test"
			},
		},
		{
			name:    "invalid JSON",
			input:   "this is not json",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var result generator.ArticleCritique
			err := ai.ParseJSON(tc.input, &result)
			if tc.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tc.wantErr && tc.check != nil && !tc.check(&result) {
				t.Errorf("result check failed for input: %q, got: %+v", tc.input, result)
			}
		})
	}
}
