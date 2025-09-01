# Year-prefixed sheet names (base names in env)

Status: Accepted

Context
- In precedenza le variabili d'ambiente contenevano i nomi completi dei fogli di Google con l'anno (es. "2025 Expenses"). Questo richiedeva aggiornamenti manuali annuali e aumentava il rischio di incongruenze tra fogli (expenses, dashboard, categorie, sottocategorie).
- Il codice del reader dashboard aveva già un concetto di prefisso/pattern, ma l'esperienza di configurazione non era uniforme tra i vari fogli.

Decisione
- Le variabili d'ambiente ora accettano i nomi base senza anno:
  - `GOOGLE_SHEET_NAME` (es. `Expenses`)
  - `GOOGLE_CATEGORIES_SHEET_NAME` (es. `Dashboard`)
  - `GOOGLE_SUBCATEGORIES_SHEET_NAME` (es. `Dashboard`)
  - `DASHBOARD_SHEET_NAME` (es. `Dashboard`)
- L'applicazione prefigge automaticamente l'anno corrente ai nomi base al boot ("<anno> <base>"), evitando cambi manuali ogni anno.
- Compatibilità legacy: se `DASHBOARD_SHEET_NAME` non è impostato, resta supportato `DASHBOARD_SHEET_PREFIX` (es. "%d Dashboard").
- Salvaguardia: se un nome base inizia già con un anno a 4 cifre ("YYYY "), non viene ri-prefissato.

Conseguenze
- Positivi:
  - Config più semplice: si imposta una volta sola il nome base.
  - Meno errori di configurazione e passaggi a inizio anno.
  - Comportamento uniforme per tutti i fogli.
- Negativi / Trade-off:
  - Il binding avviene al boot: l'istanza di runtime punta ai fogli dell'anno in corso (coerente con l'uso atteso). Per leggere anni diversi, le API espongono già parametri di anno dove necessario (es. dashboard).
  - Ambienti che già specificano nomi con anno non necessitano cambi, ma è consigliato migrare ai nomi base per coerenza.

