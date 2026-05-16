// Package render owns HTML template parsing and rendering. It is a shared
// utility; feature slices interact with it via the small Renderer
// interface they declare locally.
//
// Layout convention:
//   - templates/layouts/base.html defines {{define "base"}} and includes
//     {{block "content" .}}{{end}} where pages plug in their body.
//   - each page lives at templates/<slice>/<name>.html and overrides
//     "content" with {{define "content"}}...{{end}}.
//   - the page is rendered by name "<slice>/<name>".
package render

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
)

// Templates holds one parsed *template.Template per page, each cloned from
// the shared layout set.
type Templates struct {
	pages map[string]*template.Template
}

// Load parses templates from the given filesystem rooted at "templates/".
func Load(tfs fs.FS) (*Templates, error) {
	layoutSet, err := template.New("").Funcs(funcs).ParseFS(tfs, "templates/layouts/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse layouts: %w", err)
	}

	pages := make(map[string]*template.Template)
	err = fs.WalkDir(tfs, "templates", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		rel := strings.TrimPrefix(path, "templates/")
		if strings.HasPrefix(rel, "layouts/") || strings.HasPrefix(rel, "components/") {
			return nil
		}
		name := strings.TrimSuffix(rel, ".html")
		clone, err := layoutSet.Clone()
		if err != nil {
			return err
		}
		t, err := clone.ParseFS(tfs, path)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		// Pull in shared components, if any.
		if hasDir(tfs, "templates/components") {
			if _, err := t.ParseFS(tfs, "templates/components/*.html"); err != nil {
				return fmt.Errorf("parse components: %w", err)
			}
		}
		pages[name] = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Templates{pages: pages}, nil
}

// Render writes the named page using the base layout.
func (t *Templates) Render(w http.ResponseWriter, name string, data any) error {
	tmpl, ok := t.pages[name]
	if !ok {
		return fmt.Errorf("unknown template %q", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, "base", data)
}

func hasDir(tfs fs.FS, path string) bool {
	d, err := fs.ReadDir(tfs, path)
	return err == nil && len(d) > 0
}
