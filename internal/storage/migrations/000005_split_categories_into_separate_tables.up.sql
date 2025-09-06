-- Create separate tables for primary and secondary categories

-- Primary categories table
CREATE TABLE primary_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Secondary categories table with foreign key to primary
CREATE TABLE secondary_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    primary_category_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (primary_category_id) REFERENCES primary_categories(id) ON DELETE CASCADE
);

-- Create indexes for better performance
CREATE INDEX idx_secondary_categories_primary_id ON secondary_categories(primary_category_id);
CREATE INDEX idx_primary_categories_name ON primary_categories(name);
CREATE INDEX idx_secondary_categories_name ON secondary_categories(name);

-- Insert primary categories
INSERT INTO primary_categories (id, name) VALUES
(1, 'Casa'),
(2, 'Salute'),
(3, 'Spesa'),
(4, 'Trasporti'),
(5, 'Fuori (come fuori a cena...)'),
(6, 'Viaggi'),
(7, 'Bimbi'),
(8, 'Vestiti'),
(9, 'Divertimento'),
(10, 'Regali'),
(11, 'Tasse e Percentuali'),
(12, 'Altre spese'),
(13, 'Lavoro');

-- Insert secondary categories with proper relationships

-- Casa (id=1)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Mutuo', 1),
('Spese condominiali', 1),
('Internet', 1),
('Mobili', 1),
('Assicurazioni', 1),
('Pulizia', 1),
('Elettricità', 1),
('Telefono', 1);

-- Salute (id=2)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Assicurazione sanitaria', 2),
('Dottori', 2),
('Medicine', 2),
('Personale', 2),
('Sport', 2);

-- Spesa (id=3)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Everli', 3),
('Altre spese (non Everli)', 3);

-- Trasporti (id=4)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Trasporto locale', 4),
('Car sharing', 4),
('Spese automobile', 4),
('Servizi taxi', 4);

-- Fuori (come fuori a cena...) (id=5)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Ristoranti', 5),
('Bar', 5),
('Cibo a casa', 5);

-- Viaggi (id=6)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Vacanza', 6),
('Vacanza estiva', 6);

-- Bimbi (id=7)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Cura bimbi', 7),
('Roba bimbi', 7),
('Corsi bimbi', 7),
('Baby sitter', 7);

-- Vestiti (id=8)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Vestiti e', 8),
('Vestiti g', 8),
('Vestiti bimbi', 8);

-- Divertimento (id=9)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Tech', 9),
('Libri e', 9),
('Divertimento e', 9),
('Learning e', 9),
('Giochi e', 9),
('Giochi g', 9),
('Learning g', 9),
('Divertimento familiare', 9),
('Altri divertimenti', 9);

-- Regali (id=10)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Altri regali', 10);

-- Tasse e Percentuali (id=11)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Brokers', 11),
('Banche', 11),
('Consulting', 11),
('Altre tasse e percentuali', 11);

-- Altre spese (id=12)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Tasse statali', 12),
('2DM', 12),
('Unknown', 12);

-- Lavoro (id=13)
INSERT INTO secondary_categories (name, primary_category_id) VALUES
('Lavoro g', 13),
('Lavoro e', 13);

-- Drop the old categories table since we're replacing it with the new structure
DROP TABLE categories;