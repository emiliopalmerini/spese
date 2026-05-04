-- Net Worth: accounts and monthly balances
CREATE TABLE accounts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL CHECK (type IN ('cash','rainy_day','long_term')),
    active      INTEGER NOT NULL DEFAULT 1,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_accounts_type ON accounts(type);

CREATE TABLE account_balances (
    account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    year          INTEGER NOT NULL,
    month         INTEGER NOT NULL CHECK (month BETWEEN 1 AND 12),
    amount_cents  INTEGER NOT NULL,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, year, month)
);

CREATE INDEX idx_account_balances_period ON account_balances(year, month);
