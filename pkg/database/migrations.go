package database

import (
	"context"
	"fmt"
)

func (db *DB) Migrate(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS profiles (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`INSERT INTO profiles (name)
		 SELECT 'Shared Garden'
		 WHERE NOT EXISTS (SELECT 1 FROM profiles)`,
		`CREATE TABLE IF NOT EXISTS plants (
			id SERIAL PRIMARY KEY,
			profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
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
		`CREATE INDEX IF NOT EXISTS idx_plants_profile_id ON plants(profile_id)`,
		// Migrations for existing tables
		`DO $$ BEGIN
			ALTER TABLE plants ADD COLUMN IF NOT EXISTS profile_id INTEGER;
		EXCEPTION WHEN OTHERS THEN NULL;
		END $$`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'plants_profile_id_fk'
			) THEN
				ALTER TABLE plants
				ADD CONSTRAINT plants_profile_id_fk
				FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE;
			END IF;
		EXCEPTION WHEN OTHERS THEN NULL;
		END $$`,
		`DO $$ BEGIN
			IF EXISTS (
				SELECT 1
				FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'plants'
				  AND column_name = 'profile_id'
			) THEN
				UPDATE plants
				SET profile_id = (
					SELECT id FROM profiles ORDER BY id ASC LIMIT 1
				)
				WHERE profile_id IS NULL;

				BEGIN
					ALTER TABLE plants ALTER COLUMN profile_id SET NOT NULL;
				EXCEPTION WHEN OTHERS THEN NULL;
				END;
			END IF;
		EXCEPTION WHEN OTHERS THEN NULL;
		END $$`,
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
