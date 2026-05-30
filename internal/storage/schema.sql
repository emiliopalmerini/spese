CREATE TABLE IF NOT EXISTS accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  type TEXT NOT NULL CHECK (type IN ('Asset', 'Liability')),
  class TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'EUR',
  active_from TEXT NOT NULL DEFAULT '',
  active_to TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS transactions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  date TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('Income', 'Expense', 'Transfer', 'Adjustment')),
  account TEXT NOT NULL REFERENCES accounts(name) ON UPDATE CASCADE,
  amount_cents INTEGER NOT NULL,
  category TEXT NOT NULL DEFAULT '',
  subcategory TEXT NOT NULL DEFAULT '',
  payee TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_transactions_date ON transactions(date);
CREATE INDEX IF NOT EXISTS idx_transactions_account ON transactions(account);
CREATE INDEX IF NOT EXISTS idx_transactions_kind ON transactions(kind);

CREATE TABLE IF NOT EXISTS snapshot_batches (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  effective_month TEXT NOT NULL,
  captured_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  note TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_snapshot_batches_month ON snapshot_batches(effective_month);

CREATE TABLE IF NOT EXISTS snapshot_balances (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_id INTEGER NOT NULL REFERENCES snapshot_batches(id) ON DELETE CASCADE,
  account TEXT NOT NULL REFERENCES accounts(name) ON UPDATE CASCADE,
  balance_cents INTEGER NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  UNIQUE (batch_id, account)
);

CREATE INDEX IF NOT EXISTS idx_snapshot_balances_account ON snapshot_balances(account);
