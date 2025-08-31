# Visualizzare le spese mensili sotto il form

Status: Accepted

Context
- Vogliamo mostrare un riepilogo delle spese del mese corrente direttamente sotto al form di inserimento spese (HTMX) per fornire feedback immediato e contesto.
- Dallo spreadsheet esiste (o può esistere) un foglio di dashboard annuale (es. "2025 Dashboard") che espone, in forma tabellare, i totali mensili per categoria primaria e le relative sottocategorie. Esempio (CSV semplificato): colonne `Primary,Secondary,Jan,Feb,...,Dec,Average,Total` e righe per categorie (Primary), righe per sottocategorie (Secondary) e una riga finale `total` con i totali per mese.
- Alternative: calcolare i totali mensili scansionando la sheet delle transazioni/expenses filtrando per mese ogni volta, con maggiore costo e rischio di incoerenze rispetto alle logiche del foglio di calcolo.

Decisione
- Sotto al form verrà reso un widget "Panoramica mese" con:
  - Totale del mese corrente.
  - Breakdown per categoria primaria (righe dove `Secondary` è vuoto) con importo del mese corrente e una barra/progress visuale CSS-only.
- Sorgente dati principale: il foglio di dashboard annuale configurabile, di default denominato `"%d Dashboard"` (es. `2025 Dashboard`).
  - Parsing: si legge un range ampio (es. `A1:Q200`) e si valida l'intestazione: `Primary`, `Secondary`, i 12 mesi (abbreviazioni inglesi: `Jan..Dec`), opzionali `Average`, `Total`.
  - Mappatura mesi: il server mappa il mese corrente al corrispondente header (es. `Jan`=1, `Feb`=2, ...). La localizzazione degli header è configurabile ma per default si usano abbreviazioni inglesi.
  - Categoria: includiamo nel widget solo le righe in cui `Primary` valorizzato e `Secondary` vuoto (totali per categoria). Le sottocategorie sono ignorate nel widget per compattezza; i loro valori sono già inclusi nei totali di categoria.
  - Totale mese: letto dalla riga `total` (prima colonna vuota o "total" nella colonna `Primary`), se presente; in alternativa somma dei totali categoria del mese.
- Fallback robusto: se il foglio dashboard non è presente oppure l'header atteso non è valido, il server ricalcola i totali mensili scansionando la sheet delle spese/transazioni filtrando per anno+mese.
- Caching e aggiornamento:
  - Il riepilogo viene cache-ato in memoria per 5 minuti per ridurre roundtrip verso Google Sheets.
  - Dopo un inserimento spesa andato a buon fine, il server invalida la cache del mese corrente e serve il parziale aggiornato (o applica un aggiornamento ottimistico se disponibile l'importo e la categoria).
- API/UI:
  - Endpoint HTMX: `GET /ui/month-overview?year=YYYY&month=MM` che restituisce un partial HTML (`text/html`) con tabella + progress bars.
  - Il partial è incluso nella pagina principale sotto il form. Il form, al `POST` riuscito, chiede anche il refresh del widget (es. `hx-trigger` custom o risposta che aggiorna il target del widget via `hx-swap`).
  - Template: `web/templates/partials/month_overview.html` con layout SSR minimal, nessun JS custom oltre HTMX.
- Configurazione:
  - `DASHBOARD_SHEET_PREFIX` (default: `%d Dashboard`).
  - `DASHBOARD_MONTH_HEADERS` (default: `Jan,Feb,Mar,Apr,May,Jun,Jul,Aug,Sep,Oct,Nov,Dec`).
  - `SUMMARY_CACHE_TTL` (default: 5m).
  - Flag facoltativo `USE_DASHBOARD_SUMMARY=true` (default: true) per forzare fallback.

Conseguenze
- Positivi:
  - Prestazioni: un'unica lettura del foglio dashboard e rendering SSR; meno calcoli lato server.
  - Coerenza: i numeri mostrati coincidono con la logica/rounding del foglio di calcolo.
  - UX: feedback immediato dopo l'invio della spesa, con breakdown per categoria.
- Negativi:
  - Accoppiamento al layout del foglio dashboard (header/colonne): cambi del foglio possono rompere il parser.
  - Localizzazione mesi: header non inglesi richiedono configurazione esplicita.
- Mitigazioni:
  - Validazione dell'header e messaggi chiari in log se il layout non è quello atteso; fallback automatico al calcolo da transazioni.
  - Test unitari con fixture CSV simile all'esempio fornito (2025 Dashboard) per coprire parsing, mapping mesi e selezione righe categoria.
  - Caching con invalidazione post-inserimento per mantenere i dati aggiornati senza moltiplicare le chiamate API.

Note di implementazione
- `internal/sheets`: aggiungere un piccolo reader `DashboardReader` con interfaccia `ReadMonthlySummary(ctx, year int) (Summary, error)` che restituisce per mese un totale globale e una lista di pair `{category, amount}`.
- `internal/core`: definire modelli `MonthlySummary` e normalizzazione valori (es. arrotondamento a due decimali, valuta da config).
- `internal/http`: handler `MonthOverviewHandler` che compone i dati e renderizza il partial; integrazione con il form di inserimento via HTMX.
- `web/templates`: partial `partials/month_overview.html` con tabella e progress bar CSS (no JS custom), marker HTML utile ai test.
- `tests`: unit test per il parsing (fixture derivata dal CSV di esempio), e test `httptest` del handler che attesta status, content-type e presenza di marker HTML/valori chiave.

