package http

import (
    "html/template"
    "log"
    "net/http"
    "path/filepath"
    "time"
)

type Server struct {
    http.Server
    templates *template.Template
}

// NewServer configures routes and templates, returning a ready-to-run http.Server.
func NewServer(addr string) *Server {
    mux := http.NewServeMux()

    s := &Server{
        Server: http.Server{
            Addr:    addr,
            Handler: mux,
        },
    }

    // Parse templates at startup. We keep it simple for now.
    patterns := []string{
        filepath.Join("web", "templates", "*.html"),
    }
    t, err := template.ParseGlob(patterns[0])
    if err != nil {
        log.Printf("warning: failed parsing templates: %v", err)
    }
    s.templates = t

    // Static assets
    fs := http.FileServer(http.Dir("web/static"))
    mux.Handle("/static/", http.StripPrefix("/static/", fs))

    // Routes
    mux.HandleFunc("/", s.handleIndex)
    mux.HandleFunc("/healthz", handleHealth)
    mux.HandleFunc("/readyz", handleReady)

    return s
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte("ok"))
}

func handleReady(w http.ResponseWriter, r *http.Request) {
    // In futuro: aggiungere check integrazione Sheets.
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte("ready"))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
    if s.templates == nil {
        http.Error(w, "templates not loaded", http.StatusInternalServerError)
        return
    }

    now := time.Now()
    data := struct {
        Day        int
        Month      int
        Categories []string
        Subcats    []string
    }{
        Day:   now.Day(),
        Month: int(now.Month()),
        // Placeholder: valori statici finché non si integra Sheets
        Categories: []string{"Casa", "Cibo", "Trasporti"},
        Subcats:    []string{"Generale", "Supermercato", "Ristorante"},
    }

    if err := s.templates.ExecuteTemplate(w, "index.html", data); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

