# ADR 0003 — Modelli di Dominio e Dati

Status: Accepted

## Contesto

L'app registra spese con data (giorno, mese) precompilata, descrizione e importo, e legge categorie e sottocategorie dallo Spreadsheet. In questo progetto, categorie e sottocategorie sono insiemi indipendenti: qualsiasi sottocategoria può essere combinata con qualsiasi categoria (nessuna relazione padre-figlio vincolante).

## Decisione

### Entità di Dominio

- Expense
  - date: DateParts (derivata automaticamente dalla data corrente salvo override)
  - description: Description
  - amount: Money
  - category: Category
  - subcategory: Subcategory

- Category
  - name: string (univoco; mostrato all'utente)

- Subcategory
  - name: string (univoco globale; indipendente dalle categorie)

- Money
  - rappresentato in minor units (cents) come `int64` (> 0)
  - input come decimale stringa (es. "12.34"), normalizzato a cents con arrotondamento commerciale

- DateParts
  - { day int (1..31), month int (1..12) }

- Description
  - string (trimmed, max 200)

Note: niente ID interni persistiti; su Sheets l'identità è implicita (es. numero di riga) se mai servisse.

### Value Objects / DTO

- NewExpenseInput (DTO di input)
  - { day, month, description, amount_decimal, category, subcategory }
  - `amount_decimal` convertito a `amount_cents` lato dominio; day/month opzionali, default alla data di sistema.

- Taxonomy (DTO di uscita)
  - { categories: []Category, subcategories: []Subcategory }
  - usato per popolare select/HTMX.

### Schema Google Sheets

- Foglio Spese (es. `2025 Expenses`):
  - Colonne (header riga 1):
    - A: Month (numero, 1-12)
    - B: Day (numero, 1-31)
    - C: Expense (testo, descrizione)
    - D: Amount (decimale, es. 12.34)
    - E: Currency (testo, sempre "EUR")
    - F: EUR (decimale, uguale a Amount)
    - G: Primary (testo, categoria principale)
    - H: Secondary (testo, sottocategoria)
  - Range principale: `2025 Expenses!A:H` (append in coda, inclusi header).
  - Nota: internamente trattiamo l'importo in cents; su Sheets salviamo il valore decimale per leggibilità.

- Liste tassonomiche (indipendenti):
  - Foglio `2025 Dashboard`: colonna A = Category (da A2:A65), colonna B = Subcategory (da B2:B65).
  - Range specifici: `2025 Dashboard!A2:A65` per categorie, `2025 Dashboard!B2:B65` per sottocategorie.
  - Non include header rows nei range (salta automaticamente riga 1).

### Regole di Validazione

- day ∈ [1,31], month ∈ [1,12]. Se assenti nell'input, derivati dalla data corrente (timezone configurabile).
- description non vuota, ≤ 200 caratteri, trimmed.
- amount_decimal > 0; conversione a cents con arrotondamento half-up a 2 decimali.
- category deve esistere in `2025 Dashboard A2:A65` (match case-insensitive, nome canonico salvato).
- subcategory deve esistere in `2025 Dashboard B2:B65` (match case-insensitive, nome canonico salvato).
- nessun vincolo di accoppiamento categoria↔sottocategoria; qualsiasi combinazione è valida.

### Interfacce (Ports) principali

- ExpenseWriter: `Append(ctx context.Context, e Expense) (rowRef string, err error)`
  - `rowRef` opzionale: può essere vuoto o contenere, ad es., l'indice di riga restituito da Sheets.

- TaxonomyReader: `List(ctx context.Context) (cats []Category, subs []Subcategory, err error)`

## Conseguenze

- Pro:
  - Modelli semplici, adatti a storage tabellare su Sheets, con tassonomie indipendenti.
  - Conversione/validazione centralizzate nel dominio; handlers più puliti.
  - Facilità di manutenzione: aggiungere categorie/sottocategorie non richiede mapping.
- Contro:
  - Nessuna FK o vincoli forti: possibili combinazioni semanticamente inconsistenti.
  - Aggregazioni/filtri avanzati demandati all'app o a funzioni di Sheets.
