-- Revert to original English categories
DELETE FROM categories;

-- Insert original primary categories
INSERT OR IGNORE INTO categories (name, type) VALUES
('Alimentari', 'primary'),
('Trasporti', 'primary'),
('Casa', 'primary'),
('Sanità', 'primary'),
('Svago', 'primary'),
('Vestiti', 'primary'),
('Regali', 'primary'),
('Tasse', 'primary'),
('Investimenti', 'primary'),
('Altro', 'primary');

-- Insert original subcategories
INSERT OR IGNORE INTO categories (name, type) VALUES
('Supermercato', 'secondary'),
('Ristorante', 'secondary'),
('Benzina', 'secondary'),
('Trasporto Pubblico', 'secondary'),
('Affitto', 'secondary'),
('Bollette', 'secondary'),
('Medico', 'secondary'),
('Farmacia', 'secondary'),
('Cinema', 'secondary'),
('Hobby', 'secondary'),
('Abbigliamento', 'secondary'),
('Scarpe', 'secondary'),
('Compleanno', 'secondary'),
('Natale', 'secondary'),
('IRPEF', 'secondary'),
('IMU', 'secondary'),
('Azioni', 'secondary'),
('Crypto', 'secondary'),
('Varie', 'secondary');