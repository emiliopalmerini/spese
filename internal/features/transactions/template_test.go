package transactions

import (
	"net/http/httptest"
	"strings"
	"testing"

	"spese/internal/render"
	"spese/web"
)

func TestListTemplateKeepsFreeTextCategoryWithRankedSuggestions(t *testing.T) {
	templates, err := render.Load(web.TemplatesFS)
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}
	w := httptest.NewRecorder()
	view := ListView{
		Filters: ListFilterView{Category: "Imported"},
		CategorySuggestions: []CategorySuggestion{
			{Name: "Cibo", Count: 3},
			{Name: "Casa", Count: 2},
		},
	}

	if err := templates.Render(w, "transactions/list", view); err != nil {
		t.Fatalf("render list: %v", err)
	}
	body := w.Body.String()
	if !strings.Contains(body, `input type="text" name="category" value="Imported" list="transaction-category-suggestions" autocomplete="off"`) {
		t.Fatalf("category filter is not a free-text input connected to suggestions: %s", body)
	}
	first := strings.Index(body, `<option value="Cibo">Cibo (3)</option>`)
	second := strings.Index(body, `<option value="Casa">Casa (2)</option>`)
	if first == -1 || second == -1 || first >= second {
		t.Fatalf("category suggestions are missing or out of order: %s", body)
	}
}
