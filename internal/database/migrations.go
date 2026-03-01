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
			health_score INTEGER CHECK (health_score BETWEEN 1 AND 10),
			diagnosis TEXT,
			care_tips TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_assessments_plant_id ON assessments(plant_id)`,
	}

	for _, q := range queries {
		if _, err := db.Pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("running migration: %w", err)
		}
	}
	return nil
}
