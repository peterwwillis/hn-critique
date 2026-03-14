// Package generator writes the static HTML site from story data.
package generator

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Generator writes static HTML pages to outputDir.
type Generator struct {
	outputDir string
	tmpl      *template.Template
}

// New creates a Generator that writes pages into outputDir.
func New(outputDir string) *Generator {
	return &Generator{outputDir: outputDir}
}

// Generate writes all static pages for the given stories.
func (g *Generator) Generate(stories []*Story) error {
	if err := g.loadTemplates(); err != nil {
		return fmt.Errorf("loading templates: %w", err)
	}

	for _, dir := range []string{
		g.outputDir,
		CacheDir(g.outputDir),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	// Copy static assets.
	if err := g.copyStatic(); err != nil {
		return fmt.Errorf("copying static assets: %w", err)
	}

	// Write .nojekyll so GitHub Pages doesn't process the files.
	nojekyll := filepath.Join(g.outputDir, ".nojekyll")
	if err := os.WriteFile(nojekyll, nil, 0o644); err != nil {
		return err
	}

	generatedAt := time.Now().UTC().Format("2006-01-02 15:04 UTC")

	for _, s := range stories {
		datePath := storyDatePath(s.Time)
		s.CritiquePath = path.Join("critique", datePath, fmt.Sprintf("%d.html", s.ID))
		s.CommentsPath = path.Join("comments", datePath, fmt.Sprintf("%d.html", s.ID))
	}

	if err := g.writeTemplate("index.html", filepath.Join(g.outputDir, "index.html"),
		IndexData{Stories: stories, GeneratedAt: generatedAt, RootPath: ""}); err != nil {
		return err
	}

	for _, s := range stories {
		datePath := storyDatePath(s.Time)
		rootPath := rootPathForDatePath(datePath)
		data := PageData{Story: s, GeneratedAt: generatedAt, RootPath: rootPath}

		critiquePath := filepath.Join(g.outputDir, filepath.FromSlash(s.CritiquePath))
		if err := os.MkdirAll(filepath.Dir(critiquePath), 0o755); err != nil {
			return fmt.Errorf("critique dir %d: %w", s.ID, err)
		}
		if err := g.writeTemplate("critique.html", critiquePath, data); err != nil {
			return fmt.Errorf("critique page %d: %w", s.ID, err)
		}

		commentsPath := filepath.Join(g.outputDir, filepath.FromSlash(s.CommentsPath))
		if err := os.MkdirAll(filepath.Dir(commentsPath), 0o755); err != nil {
			return fmt.Errorf("comments dir %d: %w", s.ID, err)
		}
		if err := g.writeTemplate("comments.html", commentsPath, data); err != nil {
			return fmt.Errorf("comments page %d: %w", s.ID, err)
		}
	}
	return nil
}

// ---- template data types ----

// IndexData is passed to the index template.
type IndexData struct {
	Stories     []*Story
	GeneratedAt string
	RootPath    string
}

// PageData is passed to the critique and comments templates.
type PageData struct {
	Story       *Story
	GeneratedAt string
	RootPath    string
}

// ---- internal helpers ----

func (g *Generator) loadTemplates() error {
	caser := cases.Title(language.English)
	funcs := template.FuncMap{
		"ago":      timeAgo,
		"safeHTML": func(s string) template.HTML { return template.HTML(s) }, //nolint:gosec // comment HTML is from HN API
		"title":    caser.String,
		"join":     func(sep string, s []string) string { return joinStrings(sep, s) },
		"mul":      func(a, b int) int { return a * b },
		"ratingClass": func(r string) string {
			switch r {
			case "reliable":
				return "rating-reliable"
			case "misleading":
				return "rating-misleading"
			default:
				return "rating-questionable"
			}
		},
	}

	sub, err := fs.Sub(embeddedFS, "templates")
	if err != nil {
		return fmt.Errorf("opening embedded templates: %w", err)
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(sub, "*.html")
	if err != nil {
		return err
	}
	g.tmpl = tmpl
	return nil
}

func (g *Generator) writeTemplate(name, outPath string, data any) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return g.tmpl.ExecuteTemplate(f, name, data)
}

// copyStatic copies everything from the embedded static/ dir into outputDir.
func (g *Generator) copyStatic() error {
	return fs.WalkDir(embeddedFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("static", path)
		dst := filepath.Join(g.outputDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		return copyEmbedFile(embeddedFS, path, dst)
	})
}

func copyEmbedFile(fsys fs.FS, src, dst string) error {
	in, err := fsys.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// timeAgo returns a human-readable duration string for a Unix timestamp.
func timeAgo(t int64) string {
	d := time.Since(time.Unix(t, 0))
	switch {
	case d.Hours() > 24*30:
		return fmt.Sprintf("%.0f months ago", d.Hours()/(24*30))
	case d.Hours() > 24:
		return fmt.Sprintf("%.0f days ago", d.Hours()/24)
	case d.Hours() >= 2:
		return fmt.Sprintf("%.0f hours ago", d.Hours())
	case d.Hours() >= 1:
		return "1 hour ago"
	case d.Minutes() >= 2:
		return fmt.Sprintf("%.0f minutes ago", d.Minutes())
	default:
		return "just now"
	}
}

// joinStrings joins a slice of strings with a separator.
func joinStrings(sep string, s []string) string {
	result := ""
	for i, v := range s {
		if i > 0 {
			result += sep
		}
		result += v
	}
	return result
}

func storyDatePath(storyTime int64) string {
	return time.Unix(storyTime, 0).UTC().Format("2006/01/02")
}

func rootPathForDatePath(datePath string) string {
	parts := strings.Split(datePath, "/")
	depth := 1 + len(parts)
	return strings.Repeat("../", depth)
}
