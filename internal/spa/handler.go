// Package spa serves the embedded Vite application without masking API or
// operational endpoints.
package spa

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

type Handler struct {
	files fs.FS
}

func New(embedded fs.FS) (*Handler, error) {
	files, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, err
	}
	return &Handler{files: files}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || strings.HasPrefix(r.URL.Path, "/ws/") {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		h.serveAsset(w, r)
		return
	}
	if !applicationRoute(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	content, err := fs.ReadFile(h.files, "index.html")
	if err != nil {
		http.Error(w, "frontend build missing", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (h *Handler) serveAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if !strings.HasPrefix(name, "assets/") {
		http.NotFound(w, r)
		return
	}
	content, err := fs.ReadFile(h.files, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func applicationRoute(value string) bool {
	switch strings.TrimSuffix(value, "/") {
	case "", "/movimenti", "/analisi", "/ricorrenti", "/conti", "/categorie", "/impostazioni":
		return true
	default:
		return false
	}
}
