ALTER TABLE movements ADD COLUMN description TEXT NOT NULL DEFAULT '';

CREATE INDEX movements_description_idx ON movements(lower(description), business_date DESC);
