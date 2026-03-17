package main

import (
"html/template"
"log"
"os"

"github.com/peterwwillis/hn-critique/internal/generator"
)

func main() {
outDir := "/tmp/hn-shot/site"
_ = os.RemoveAll(outDir)
gen := generator.New(outDir)
stories := []*generator.Story{{
ID:           12345,
Rank:         1,
Title:        "Go 1.24 Released",
URL:          "https://go.dev/blog/go1.24",
Domain:       "go.dev",
Score:        500,
Author:       "gopher",
Time:         1741723200,
CommentCount: 150,
Critique: &generator.ArticleCritique{Summary: "x", Truthfulness: "x", Rating: "needs citation"},
CommentsCritique: &generator.CommentsCritique{
Summary: "Discussion is mixed.",
Comments: []generator.AnalyzedComment{
{ID: 1, Author: "alice", Text: "Helpful take", Indicators: []string{"thoughtful", "constructive"}, AccuracyRank: 1, Analysis: "High signal."},
{ID: 2, Author: "bob", Text: "I doubt this claim", Indicators: []string{"likely-untrue"}, AccuracyRank: 2, Analysis: "Needs evidence."},
{ID: 3, Author: "carol", Text: "You are all idiots", Indicators: []string{"belligerent"}, AccuracyRank: 3, Analysis: "Hostile."},
},
},
Comments: []*generator.Comment{{ID: 90, Author: "raw", Text: template.HTML("<p>raw</p>"), Depth: 0}},
}}
if err := gen.Generate(stories); err != nil {
log.Fatal(err)
}
}
