# ADR-0022: Spese v2 ledger e cutover React

## Stato

Accepted, 2026-08-02.

Supersedes ADR-0020 e tutte le decisioni di UI/route degli ADR-0008–0019 incompatibili con questo documento. ADR-0021 resta valido per il transactional outbox, RabbitMQ e il mirror derivato.

## Contesto

Il modello precedente rappresentava movimenti e snapshot con nomi di conti/categorie come identità, non aveva migrazioni versionate e serviva template HTMX. Questo impediva edit/void sicuri, split canonici, ricorrenze idempotenti e riconciliazioni con semantica deterministica.

## Decisione

### Sorgente dati

SQLite è l’unica fonte di verità. Google Sheets non viene interrogato da saldo, overview o analisi. È un export completo ricostruibile e idempotente inviato tramite l’outbox di ADR-0021.

### Ledger

Un `movement` è l’evento logico e i `postings` sono le variazioni firmate dei conti. Le allocazioni collegano integralmente spese, entrate e rimborsi a categorie canoniche. ID pubblici e foreign key sono UUID; i nomi restano attributi modificabili.

Draft e void non incidono sui saldi. Un trasferimento ha esattamente due posting su conti distinti, somma zero e nessuna allocazione. Il rimborso accredita il conto ma viene sottratto dalle spese della categoria, non sommato alle entrate.

### Riconciliazioni

Una riconciliazione è un anchor autorevole, non un movimento. Il saldo a `t` usa l’ultimo `actual_balance` con `closed_through <= t` e somma solo posting successivi. In assenza di anchor usa saldo e data iniziali del conto. Questa separazione permette a un movimento retrodatato di cambiare l’analisi storica senza alterare il saldo successivo all’anchor.

Gli snapshot legacy con chiave `YYYY-MM` sono documentati nel codice storico come saldi di fine mese, non valori del primo giorno. La migrazione usa quindi l’ultimo giorno calendario reale.

### Ricorrenze

Occurrence, movimento, posting, allocazioni e avanzamento di `next_due` condividono una transazione. Il vincolo `(rule_id, scheduled_for)` rende il catch-up idempotente. Le scadenze mensili 29–31 usano l’ultimo giorno valido. Le modifiche a una regola non riscrivono il passato.

### Migrazioni

Le migrazioni SQL sono numerate, registrate in `schema_migrations` e applicate in una transazione. Prima della conversione si crea un backup SQLite consistente. Le tabelle esistenti vengono rinominate `legacy_*`; gli ID migrati sono UUID v5 deterministici. I trasferimenti ambigui bloccano l’intera migrazione.

Il tool `spese-migrate` esegue dry-run su copia, report di validazione, applicazione con backup e restore che preserva a sua volta il file corrente.

### API

La SPA usa solo `/api/v1`. Le creazioni richiedono `Idempotency-Key`; gli aggiornamenti e gli archive/void richiedono `If-Match`. Gli errori hanno envelope stabile e request ID. Le liste usano cursor pagination e filtri espliciti.

### Frontend

React/TypeScript/Vite è l’unica UI. Il build è incorporato nel binario Go; HTML, API e WebSocket hanno la stessa origine. Gli asset hashati hanno cache lunga, `index.html` no-store. Il fallback SPA è limitato alle route applicative note.

Il sistema visivo è warm-light, con background crema, superfici chiare, foreground carbone e accento giallo con testo nero. shadcn/ui è l’unica base componenti. Grafici Recharts hanno tooltip, focus e tabella equivalente.

### Sicurezza

Il prodotto resta single-user. Non viene introdotto un sistema utenti incompleto: il default ascolta su loopback e il deployment remoto dipende esplicitamente da TLS e autenticazione del reverse proxy. Mutation e WebSocket applicano same-origin check; le mutation richiedono anche un header non-simple CSRF. Sono applicati body limit, timeout, security headers, request ID e validazione audio.

## Conseguenze

- Il cutover UI è big-bang; non esiste una route per scegliere il frontend precedente.
- Il worker resta un processo separato ma usa lo stesso codice/immagine e lo stesso SQLite persistente.
- I report Sheets personalizzati devono leggere i nuovi tab derivati.
- Multi-valuta, budget, utenti e sincronizzazione bancaria restano fuori scope.
- Le tabelle `legacy_*` vengono conservate fino alla validazione post-cutover e rimosse soltanto con una decisione operativa esplicita.
