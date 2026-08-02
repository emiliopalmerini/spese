CREATE TABLE accounts (
  id TEXT PRIMARY KEY NOT NULL DEFAULT (lower(hex(randomblob(16)))),
  name TEXT NOT NULL UNIQUE,
  type TEXT NOT NULL CHECK (type IN ('Asset', 'Liability')),
  class TEXT NOT NULL CHECK (class IN ('Cash', 'Investment', 'Property', 'Tax', 'Credit', 'Other')),
  currency TEXT NOT NULL DEFAULT 'EUR' CHECK (currency = 'EUR'),
  initial_balance_cents INTEGER NOT NULL DEFAULT 0,
  initial_date TEXT NOT NULL DEFAULT (date('now')) CHECK (initial_date = date(initial_date)),
  active_from TEXT NOT NULL DEFAULT '',
  active_to TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  archived_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0)
);

CREATE UNIQUE INDEX accounts_name_unique ON accounts(lower(name));

CREATE TABLE categories (
  id TEXT PRIMARY KEY NOT NULL,
  parent_id TEXT REFERENCES categories(id) ON DELETE RESTRICT,
  kind TEXT NOT NULL CHECK (kind IN ('expense', 'income')),
  name TEXT NOT NULL,
  icon TEXT NOT NULL DEFAULT 'shapes',
  color TEXT NOT NULL DEFAULT '#725B86',
  sort_order INTEGER NOT NULL DEFAULT 0,
  archived_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE UNIQUE INDEX categories_name_unique
ON categories(kind, ifnull(parent_id, ''), lower(name));
CREATE INDEX categories_parent_idx ON categories(parent_id, sort_order, name);

CREATE TRIGGER categories_max_two_levels_insert
BEFORE INSERT ON categories WHEN NEW.parent_id IS NOT NULL
BEGIN
  SELECT CASE WHEN EXISTS (
    SELECT 1 FROM categories parent WHERE parent.id = NEW.parent_id AND parent.parent_id IS NOT NULL
  ) THEN RAISE(ABORT, 'category_depth') END;
  SELECT CASE WHEN EXISTS (
    SELECT 1 FROM categories parent WHERE parent.id = NEW.parent_id AND parent.kind <> NEW.kind
  ) THEN RAISE(ABORT, 'category_kind') END;
END;

CREATE TRIGGER categories_max_two_levels_update
BEFORE UPDATE OF parent_id, kind ON categories
BEGIN
  SELECT CASE WHEN NEW.parent_id IS NOT NULL AND EXISTS (
    SELECT 1 FROM categories parent WHERE parent.id = NEW.parent_id AND parent.parent_id IS NOT NULL
  ) THEN RAISE(ABORT, 'category_depth') END;
  SELECT CASE WHEN NEW.parent_id IS NOT NULL AND EXISTS (
    SELECT 1 FROM categories parent WHERE parent.id = NEW.parent_id AND parent.kind <> NEW.kind
  ) THEN RAISE(ABORT, 'category_kind') END;
  SELECT CASE WHEN NEW.parent_id IS NOT NULL AND EXISTS (
    SELECT 1 FROM categories child WHERE child.parent_id = NEW.id
  ) THEN RAISE(ABORT, 'category_depth') END;
END;

CREATE TABLE movements (
  id TEXT PRIMARY KEY NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('expense', 'income', 'refund', 'transfer', 'adjustment')),
  status TEXT NOT NULL CHECK (status IN ('draft', 'posted', 'void')),
  business_date TEXT NOT NULL CHECK (business_date = date(business_date)),
  amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
  merchant TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  origin TEXT NOT NULL DEFAULT 'manual' CHECK (origin IN ('manual', 'recurring', 'dictation', 'migration')),
  recurring_occurrence_id TEXT,
  voided_at TEXT NOT NULL DEFAULT '',
  void_reason TEXT NOT NULL DEFAULT '',
  legacy_source TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0)
);

CREATE INDEX movements_date_idx ON movements(business_date DESC, id DESC);
CREATE INDEX movements_status_idx ON movements(status, business_date DESC);
CREATE INDEX movements_merchant_idx ON movements(lower(merchant), business_date DESC);

CREATE TABLE postings (
  id TEXT PRIMARY KEY NOT NULL,
  movement_id TEXT NOT NULL REFERENCES movements(id) ON DELETE CASCADE,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  amount_cents INTEGER NOT NULL CHECK (amount_cents <> 0),
  created_at TEXT NOT NULL,
  UNIQUE (movement_id, account_id)
);

CREATE INDEX postings_account_idx ON postings(account_id, movement_id);

CREATE TABLE movement_allocations (
  id TEXT PRIMARY KEY NOT NULL,
  movement_id TEXT NOT NULL REFERENCES movements(id) ON DELETE CASCADE,
  category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
  amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
  created_at TEXT NOT NULL,
  UNIQUE (movement_id, category_id)
);

CREATE INDEX movement_allocations_category_idx ON movement_allocations(category_id, movement_id);

CREATE TABLE merchant_rules (
  id TEXT PRIMARY KEY NOT NULL,
  merchant TEXT NOT NULL,
  merchant_normalized TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('expense', 'income', 'refund')),
  account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
  category_id TEXT REFERENCES categories(id) ON DELETE SET NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  archived_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0)
);

CREATE UNIQUE INDEX merchant_rules_unique
ON merchant_rules(merchant_normalized, kind) WHERE archived_at = '';

CREATE TABLE reconciliation_batches (
  id TEXT PRIMARY KEY NOT NULL,
  period TEXT NOT NULL CHECK (period GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]'),
  status TEXT NOT NULL CHECK (status IN ('preview', 'committed')),
  created_at TEXT NOT NULL,
  committed_at TEXT NOT NULL DEFAULT '',
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0)
);

CREATE TABLE account_reconciliations (
  id TEXT PRIMARY KEY NOT NULL,
  batch_id TEXT NOT NULL REFERENCES reconciliation_batches(id) ON DELETE CASCADE,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  closed_through TEXT NOT NULL CHECK (closed_through = date(closed_through)),
  expected_balance_cents INTEGER NOT NULL,
  actual_balance_cents INTEGER NOT NULL,
  difference_cents INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (batch_id, account_id)
);

CREATE INDEX account_reconciliations_anchor_idx
ON account_reconciliations(account_id, closed_through DESC, created_at DESC);

CREATE TABLE recurring_rules (
  id TEXT PRIMARY KEY NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('expense', 'income')),
  frequency TEXT NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly', 'yearly')),
  interval_count INTEGER NOT NULL CHECK (interval_count > 0),
  start_date TEXT NOT NULL CHECK (start_date = date(start_date)),
  end_date TEXT CHECK (end_date IS NULL OR end_date = date(end_date)),
  day_of_month INTEGER CHECK (day_of_month IS NULL OR day_of_month BETWEEN 1 AND 31),
  timezone TEXT NOT NULL,
  amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
  amount_mode TEXT NOT NULL CHECK (amount_mode IN ('fixed', 'variable')),
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
  merchant TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL CHECK (state IN ('active', 'paused', 'archived')),
  mode TEXT NOT NULL CHECK (mode IN ('auto_post', 'needs_confirmation')),
  next_due TEXT NOT NULL CHECK (next_due = date(next_due)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0)
);

CREATE INDEX recurring_rules_due_idx ON recurring_rules(state, next_due);

CREATE TABLE recurring_occurrences (
  id TEXT PRIMARY KEY NOT NULL,
  rule_id TEXT NOT NULL REFERENCES recurring_rules(id) ON DELETE RESTRICT,
  scheduled_for TEXT NOT NULL CHECK (scheduled_for = date(scheduled_for)),
  status TEXT NOT NULL CHECK (status IN ('draft', 'posted', 'skipped', 'failed')),
  amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
  amount_certainty TEXT NOT NULL CHECK (amount_certainty IN ('certain', 'estimated')),
  movement_id TEXT UNIQUE REFERENCES movements(id) ON DELETE RESTRICT,
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (rule_id, scheduled_for)
);

CREATE INDEX recurring_occurrences_date_idx ON recurring_occurrences(scheduled_for, status);

CREATE TABLE api_idempotency (
  idempotency_key TEXT NOT NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  status_code INTEGER NOT NULL,
  response_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (idempotency_key, method, path)
);

CREATE TABLE sheet_sync_outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  scope TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'publishing', 'published')),
  attempts INTEGER NOT NULL DEFAULT 0,
  available_at TEXT NOT NULL,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  published_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_sheet_sync_outbox_claim
ON sheet_sync_outbox(status, available_at, id);
