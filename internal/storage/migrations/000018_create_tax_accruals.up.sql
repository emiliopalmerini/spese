CREATE TABLE tax_accruals (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    income_id       INTEGER NOT NULL REFERENCES incomes(id) ON DELETE CASCADE,
    tax_code        TEXT NOT NULL,
    rate_basis_pts  INTEGER NOT NULL,
    amount_cents    INTEGER NOT NULL CHECK (amount_cents >= 0),
    date            DATE NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (income_id, tax_code)
);

CREATE INDEX idx_tax_accruals_date ON tax_accruals(date);
CREATE INDEX idx_tax_accruals_code_date ON tax_accruals(tax_code, date);
