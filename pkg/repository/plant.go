package repository

import (
	"context"
	"fmt"

	"github.com/andre/plantdoc/pkg/database"
	"github.com/andre/plantdoc/pkg/model"
)

type PlantRepo struct {
	db *database.DB
}

func NewPlantRepo(db *database.DB) *PlantRepo {
	return &PlantRepo{db: db}
}

func (r *PlantRepo) Create(ctx context.Context, name, species, commonName string) (*model.Plant, error) {
	var p model.Plant
	err := r.db.Pool.QueryRow(ctx,
		`INSERT INTO plants (name, species, common_name) VALUES ($1, $2, $3)
		 RETURNING id, name, species, common_name, created_at`,
		name, species, commonName,
	).Scan(&p.ID, &p.Name, &p.Species, &p.CommonName, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating plant: %w", err)
	}
	return &p, nil
}

func (r *PlantRepo) GetByID(ctx context.Context, id int) (*model.Plant, error) {
	var p model.Plant
	err := r.db.Pool.QueryRow(ctx,
		`SELECT id, name, species, common_name, created_at FROM plants WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Species, &p.CommonName, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting plant %d: %w", id, err)
	}
	return &p, nil
}

func (r *PlantRepo) List(ctx context.Context) ([]model.Plant, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, name, species, common_name, created_at FROM plants ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing plants: %w", err)
	}
	defer rows.Close()

	var plants []model.Plant
	for rows.Next() {
		var p model.Plant
		if err := rows.Scan(&p.ID, &p.Name, &p.Species, &p.CommonName, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning plant: %w", err)
		}
		plants = append(plants, p)
	}
	return plants, nil
}

func (r *PlantRepo) Delete(ctx context.Context, id int) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM plants WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting plant %d: %w", id, err)
	}
	return nil
}
