package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const storyInputFilename = "story-input.json"

// StoryInputPath returns the full path to the story input cache file.
func StoryInputPath(outputDir string) string {
	return filepath.Join(CacheDir(outputDir), storyInputFilename)
}

// SaveStoryInputs writes the raw story data used for AI analysis.
func SaveStoryInputs(outputDir string, stories []*Story) error {
	dir := CacheDir(outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(stories, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling story inputs: %w", err)
	}
	return os.WriteFile(StoryInputPath(outputDir), data, 0o644)
}

// LoadStoryInputs reads the raw story data for AI analysis.
func LoadStoryInputs(outputDir string) ([]*Story, error) {
	path := StoryInputPath(outputDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading story inputs %s: %w", path, err)
	}
	var stories []*Story
	if err := json.Unmarshal(data, &stories); err != nil {
		return nil, fmt.Errorf("parsing story inputs %s: %w", path, err)
	}
	return stories, nil
}
