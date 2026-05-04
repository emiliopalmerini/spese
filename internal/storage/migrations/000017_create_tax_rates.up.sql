CREATE TABLE tax_rates (
    code            TEXT NOT NULL,
    label           TEXT NOT NULL,
    rate_basis_pts  INTEGER NOT NULL CHECK (rate_basis_pts >= 0),
    valid_from      DATE NOT NULL,
    valid_to        DATE,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (code, valid_from)
);

CREATE INDEX idx_tax_rates_code_period ON tax_rates(code, valid_from);

CREATE TABLE freelance_income_categories (
    category TEXT PRIMARY KEY,
    active   INTEGER NOT NULL DEFAULT 1
);

INSERT INTO tax_rates (code, label, rate_basis_pts, valid_from) VALUES
('imposta_sostitutiva', 'Imposta sostitutiva',  500, '2000-01-01'),
('inps',                'INPS',                2613, '2000-01-01');

INSERT INTO freelance_income_categories (category) VALUES
('GFreelance'),
('EFreelance'),
('Freelance G'),
('Freelance E'),
('2DP+'),
('2DP');
