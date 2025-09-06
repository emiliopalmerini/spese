-- Revert back to single categories table

-- Recreate the original categories table
CREATE TABLE categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL CHECK (type IN ('primary', 'secondary')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_categories_type ON categories(type);

-- Re-insert data from the separate tables back into categories
INSERT INTO categories (name, type) 
SELECT name, 'primary' FROM primary_categories;

INSERT INTO categories (name, type) 
SELECT name, 'secondary' FROM secondary_categories;

-- Drop the separate tables
DROP TABLE secondary_categories;
DROP TABLE primary_categories;