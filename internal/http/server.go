package http

import (
    "html/template"
    "log"
    "net/http"
    "path/filepath"
    "time"
    "strconv"
    "strings"

    "spese/internal/core"
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
    mux.HandleFunc("/expenses", s.handleCreateExpense)

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

func (s *Server) handleCreateExpense(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    if err := r.ParseForm(); err != nil {
        w.WriteHeader(http.StatusBadRequest)
        _, _ = w.Write([]byte(`<div class="error">Formato richiesta non valido</div>`))
        return
    }

    now := time.Now()
    day := now.Day()
    month := int(now.Month())
    if v := strings.TrimSpace(r.Form.Get("day")); v != "" {
        if d, err := strconv.Atoi(v); err == nil {
            day = d
        }
    }
    if v := strings.TrimSpace(r.Form.Get("month")); v != "" {
        if m, err := strconv.Atoi(v); err == nil {
            month = m
        }
    }

    desc := strings.TrimSpace(r.Form.Get("description"))
    amountStr := strings.TrimSpace(r.Form.Get("amount"))
    category := strings.TrimSpace(r.Form.Get("category"))
    subcategory := strings.TrimSpace(r.Form.Get("subcategory"))

    cents, err := core.ParseDecimalToCents(amountStr)
    if err != nil {
        w.WriteHeader(http.StatusUnprocessableEntity)
        _, _ = w.Write([]byte(`<div class="error">Importo non valido</div>`))
        return
    }

    exp := core.Expense{
        Date:        core.DateParts{Day: day, Month: month},
        Description: desc,
        Amount:      core.Money{Cents: cents},
        Category:    category,
        Subcategory: subcategory,
    }
    if err := exp.Validate(); err != nil {
        w.WriteHeader(http.StatusUnprocessableEntity)
        _, _ = w.Write([]byte(`<div class="error">Dati non validi: ` + template.HTMLEscapeString(err.Error()) + `</div>`))
        return
    }

    // TODO: integrate with sheets. For now, echo success.
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte(`<div class="success">Spesa registrata: ` +
        template.HTMLEscapeString(exp.Description) +
        ` — €` + template.HTMLEscapeString(amountStr) +
        ` (` + template.HTMLEscapeString(exp.Category) + ` / ` + template.HTMLEscapeString(exp.Subcategory) + `)</div>`))
}
