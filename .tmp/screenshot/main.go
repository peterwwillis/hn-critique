package main

import (
  "fmt"
  "os"

  "github.com/peterwwillis/hn-critique/internal/generator"
)

func main() {
  outDir := "/tmp/hn-critique-sample/out"
  _ = os.RemoveAll(outDir)

  stories := []*generator.Story{{
    ID:           424242,
    Rank:         1,
    Title:        "Sample story for comment highlights",
    URL:          "https://example.com/story",
    Domain:       "example.com",
    Score:        42,
    Author:       "demo",
    Time:         1741723200,
    CommentCount: 3,
    Critique: &generator.ArticleCritique{
      Summary:      "Sample summary",
      Truthfulness: "Sample truthfulness",
      Rating:       "reliable",
    },
    CommentsCritique: &generator.CommentsCritique{
      Summary: "A mix of constructive and low-value comments.",
      Comments: []generator.AnalyzedComment{
        {ID: 1, Author: "alice", Text: "Great point and useful citation.", Indicators: []string{"constructive", "likely-true"}, AccuracyRank: 1, Analysis: "Helpful and evidence-based."},
        {ID: 2, Author: "bob", Text: "This feels overhyped and emotional.", Indicators: []string{"emotional"}, AccuracyRank: 2, Analysis: "Emotion-forward, weak support."},
        {ID: 3, Author: "carol", Text: "You are all idiots.", Indicators: []string{"belligerent", "trolling"}, AccuracyRank: 3, Analysis: "Belligerent and non-constructive."},
      },
    },
  }}

  gen := generator.New(outDir)
  if err := gen.Generate(stories); err != nil {
    panic(err)
  }

  fmt.Println("/tmp/hn-critique-sample/out/" + stories[0].CommentsPath)
}
