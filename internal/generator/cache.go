package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AnalysisCache holds cached AI analysis results for a single story.
type AnalysisCache struct {
	StoryID          int               `json:"story_id"`
	GeneratedAt      string            `json:"generated_at"`
	Critique         *ArticleCritique  `json:"critique,omitempty"`
	CommentsCritique *CommentsCritique `json:"comments_critique,omitempty"`
}

// CacheDir returns the path to the cache directory for the given output directory.
func CacheDir(outputDir string) string {
	return filepath.Join(outputDir, "cache")
}

// LoadCache loads a cached analysis for the given story ID from the cache directory.
// Returns nil (with no error) when no cache file exists for the story.
func LoadCache(outputDir string, storyID int) (*AnalysisCache, error) {
	path := filepath.Join(CacheDir(outputDir), fmt.Sprintf("%d.json", storyID))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading cache %s: %w", path, err)
	}
	var c AnalysisCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing cache %s: %w", path, err)
	}
	return &c, nil
}

// SaveCache writes an analysis cache entry for the given story ID to the cache directory.
func SaveCache(outputDir string, storyID int, cache *AnalysisCache) error {
	dir := CacheDir(outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling cache for story %d: %w", storyID, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%d.json", storyID))
	return os.WriteFile(path, data, 0o644)
}
