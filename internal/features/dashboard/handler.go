// Package dashboard renders the home page. It reads the spreadsheet's
// `dashboard` tab as-is (two columns: label / value) and turns it into a
// list of items the template can group into sections.
package dashboard

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"spese/internal/sheets"
)

// Tab is the sheet tab name for this slice.
const Tab = "dashboard"

// Item is one label-value pair from the dashboard tab. A blank Label
// indicates a section break; the renderer can use these to draw separators.
type Item struct {
	Label string
	Value string
}

// View is the template payload.
type View struct{ Items []Item }

// Renderer is the minimal template interface this slice needs.
type Renderer interface {
	Render(w http.ResponseWriter, name string, data any) error
}

// Handler renders the home page.
type Handler struct {
	Client *sheets.Client
	Logger *slog.Logger
	Render Renderer
}

// Mount registers the GET handler at prefix (use "/" for the home page).
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix, h.home)
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	items, err := readItems(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("read dashboard", "err", err)
		http.Error(w, "failed to load dashboard", http.StatusBadGateway)
		return
	}
	_ = h.Render.Render(w, "dashboard/home", View{Items: items})
}

// readItems pulls the dashboard tab as label/value pairs.
func readItems(ctx context.Context, client *sheets.Client, force bool) ([]Item, error) {
	data, err := client.ReadRange(ctx, Tab+"!A1:B", force)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", Tab, err)
	}
	out := make([]Item, 0, len(data))
	for _, r := range data {
		var label, value string
		if len(r) > 0 {
			label = sheets.CellString(r[0])
		}
		if len(r) > 1 {
			value = sheets.CellString(r[1])
		}
		out = append(out, Item{Label: label, Value: value})
	}
	return out, nil
}
