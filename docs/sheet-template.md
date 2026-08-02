# Google Sheets mirror v2

Google Sheets è un export derivato. Il worker sostituisce interamente i tab elencati sotto; non inserire dati manuali nei tab sorgente e non usarli per correggere SQLite.

## Tab

- `accounts`: identità, classificazione, saldo/data iniziali, stato e versione dei conti
- `categories`: gerarchia a due livelli, tipo, icona, colore e stato
- `movements`: evento logico, data business, tipo, stato, importo e audit
- `postings`: variazioni firmate dei conti
- `allocations`: ripartizione degli importi tra categorie
- `reconciliations`: saldo atteso/reale, differenza e `closed_through`
- `recurring_rules`: configurazione e prossima scadenza delle ricorrenze

Le intestazioni vengono create dal worker e gli importi sono centesimi interi. Le celle sono inviate con modalità `RAW`, quindi testo utente che inizia con `=` non viene interpretato come formula.

## Operatività

Il web process registra un evento nell’outbox nella stessa transazione del dato. Il relay pubblica su RabbitMQ; il worker riceve il messaggio e ricostruisce tutti i tab. Duplicati RabbitMQ sono sicuri perché l’export è idempotente.

Per provare senza Google:

```bash
docker compose up rabbitmq
make run-local
make run-worker-local
make smoke-local
```

Il file `tmp/local-sheet.json` usa le stesse righe del mirror Google.
