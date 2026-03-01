package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/andre/plantdoc/pkg/database"
	"github.com/andre/plantdoc/pkg/model"
)

type ProfileRepo struct {
	db *database.DB
}

func NewProfileRepo(db *database.DB) *ProfileRepo {
	return &ProfileRepo{db: db}
}

func (r *ProfileRepo) Create(ctx context.Context, name string) (*model.Profile, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("profile name is required")
	}

	var p model.Profile
	err := r.db.Pool.QueryRow(ctx,
		`INSERT INTO profiles (name) VALUES ($1)
		 RETURNING id, name, created_at`,
		trimmed,
	).Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating profile: %w", err)
	}
	return &p, nil
}

func (r *ProfileRepo) GetByID(ctx context.Context, id int) (*model.Profile, error) {
	var p model.Profile
	err := r.db.Pool.QueryRow(ctx,
		`SELECT id, name, created_at FROM profiles WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting profile %d: %w", id, err)
	}
	return &p, nil
}

func (r *ProfileRepo) List(ctx context.Context) ([]model.Profile, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT id, name, created_at FROM profiles ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing profiles: %w", err)
	}
	defer rows.Close()

	var profiles []model.Profile
	for rows.Next() {
		var p model.Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning profile: %w", err)
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}
