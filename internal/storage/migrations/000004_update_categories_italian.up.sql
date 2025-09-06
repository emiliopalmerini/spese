-- Clear existing categories
DELETE FROM categories;

-- Insert Italian primary categories
INSERT OR IGNORE INTO categories (name, type) VALUES
('Casa', 'primary'),
('Salute', 'primary'),
('Spesa', 'primary'),
('Trasporti', 'primary'),
('Fuori (come fuori a cena...)', 'primary'),
('Viaggi', 'primary'),
('Bimbi', 'primary'),
('Vestiti', 'primary'),
('Divertimento', 'primary'),
('Regali', 'primary'),
('Tasse e Percentuali', 'primary'),
('Altre spese', 'primary'),
('Lavoro', 'primary');

-- Insert Italian secondary categories
INSERT OR IGNORE INTO categories (name, type) VALUES
-- Casa
('Mutuo', 'secondary'),
('Spese condominiali', 'secondary'),
('Internet', 'secondary'),
('Mobili', 'secondary'),
('Assicurazioni', 'secondary'),
('Pulizia', 'secondary'),
('Elettricità', 'secondary'),
('Telefono', 'secondary'),

-- Salute
('Assicurazione sanitaria', 'secondary'),
('Dottori', 'secondary'),
('Medicine', 'secondary'),
('Personale', 'secondary'),
('Sport', 'secondary'),

-- Spesa
('Everli', 'secondary'),
('Altre spese (non Everli)', 'secondary'),

-- Trasporti
('Trasporto locale', 'secondary'),
('Car sharing', 'secondary'),
('Spese automobile', 'secondary'),
('Servizi taxi', 'secondary'),

-- Fuori (come fuori a cena...)
('Ristoranti', 'secondary'),
('Bar', 'secondary'),
('Cibo a casa', 'secondary'),

-- Viaggi
('Vacanza', 'secondary'),
('Vacanza estiva', 'secondary'),

-- Bimbi
('Cura bimbi', 'secondary'),
('Roba bimbi', 'secondary'),
('Corsi bimbi', 'secondary'),
('Baby sitter', 'secondary'),

-- Vestiti
('Vestiti e', 'secondary'),
('Vestiti g', 'secondary'),
('Vestiti bimbi', 'secondary'),

-- Divertimento
('Tech', 'secondary'),
('Libri e', 'secondary'),
('Divertimento e', 'secondary'),
('Learning e', 'secondary'),
('Giochi e', 'secondary'),
('Giochi g', 'secondary'),
('Learning g', 'secondary'),
('Divertimento familiare', 'secondary'),
('Altri divertimenti', 'secondary'),

-- Regali
('Altri regali', 'secondary'),

-- Tasse e Percentuali
('Brokers', 'secondary'),
('Banche', 'secondary'),
('Consulting', 'secondary'),
('Altre tasse e percentuali', 'secondary'),

-- Altre spese
('Tasse statali', 'secondary'),
('2DM', 'secondary'),
('Unknown', 'secondary'),

-- Lavoro
('Lavoro g', 'secondary'),
('Lavoro e', 'secondary');