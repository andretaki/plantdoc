package database

import (
	"context"
	"fmt"
)

func (db *DB) Migrate(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS plants (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			species TEXT,
			common_name TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS assessments (
			id SERIAL PRIMARY KEY,
			plant_id INTEGER NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
			photo_path TEXT NOT NULL,
			photo_data BYTEA,
			photo_mime TEXT,
			health_score INTEGER CHECK (health_score BETWEEN 1 AND 10),
			confidence TEXT,
			diagnosis TEXT,
			care_tips TEXT,
			foliage INTEGER,
			hydration INTEGER,
			pest_risk INTEGER,
			vitality INTEGER,
			urgent TEXT,
			seasonal_advice TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_assessments_plant_id ON assessments(plant_id)`,
		// Migrations for existing tables
		`DO $$ BEGIN
			ALTER TABLE assessments ADD COLUMN IF NOT EXISTS photo_data BYTEA;
			ALTER TABLE assessments ADD COLUMN IF NOT EXISTS photo_mime TEXT;
			ALTER TABLE assessments ADD COLUMN IF NOT EXISTS confidence TEXT;
			ALTER TABLE assessments ADD COLUMN IF NOT EXISTS foliage INTEGER;
			ALTER TABLE assessments ADD COLUMN IF NOT EXISTS hydration INTEGER;
			ALTER TABLE assessments ADD COLUMN IF NOT EXISTS pest_risk INTEGER;
			ALTER TABLE assessments ADD COLUMN IF NOT EXISTS vitality INTEGER;
			ALTER TABLE assessments ADD COLUMN IF NOT EXISTS urgent TEXT;
			ALTER TABLE assessments ADD COLUMN IF NOT EXISTS seasonal_advice TEXT;
		EXCEPTION WHEN OTHERS THEN NULL;
		END $$`,
	}

	for _, q := range queries {
		if _, err := db.Pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("running migration: %w", err)
		}
	}
	return nil
}
