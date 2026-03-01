package repository

import (
	"context"
	"fmt"

	"github.com/andre/plantdoc/pkg/database"
	"github.com/andre/plantdoc/pkg/model"
)

type AssessmentRepo struct {
	db *database.DB
}

func NewAssessmentRepo(db *database.DB) *AssessmentRepo {
	return &AssessmentRepo{db: db}
}

func (r *AssessmentRepo) Create(ctx context.Context, plantID int, photoPath string, photoData []byte, photoMime string, healthScore int, diagnosis, careTips string) (*model.Assessment, error) {
	var a model.Assessment
	err := r.db.Pool.QueryRow(ctx,
		`INSERT INTO assessments (plant_id, photo_path, photo_data, photo_mime, health_score, diagnosis, care_tips)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, plant_id, photo_path, health_score, diagnosis, care_tips, created_at`,
		plantID, photoPath, photoData, photoMime, healthScore, diagnosis, careTips,
	).Scan(&a.ID, &a.PlantID, &a.PhotoPath, &a.HealthScore, &a.Diagnosis, &a.CareTips, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating assessment: %w", err)
	}
	return &a, nil
}

func (r *AssessmentRepo) ListByPlant(ctx context.Context, plantID int) ([]model.Assessment, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, plant_id, photo_path, health_score, diagnosis, care_tips, created_at
		 FROM assessments WHERE plant_id = $1 ORDER BY created_at DESC`, plantID)
	if err != nil {
		return nil, fmt.Errorf("listing assessments: %w", err)
	}
	defer rows.Close()

	var assessments []model.Assessment
	for rows.Next() {
		var a model.Assessment
		if err := rows.Scan(&a.ID, &a.PlantID, &a.PhotoPath, &a.HealthScore, &a.Diagnosis, &a.CareTips, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning assessment: %w", err)
		}
		assessments = append(assessments, a)
	}
	return assessments, nil
}

func (r *AssessmentRepo) GetLatestByPlant(ctx context.Context, plantID int) (*model.Assessment, error) {
	var a model.Assessment
	err := r.db.Pool.QueryRow(ctx,
		`SELECT id, plant_id, photo_path, health_score, diagnosis, care_tips, created_at
		 FROM assessments WHERE plant_id = $1 ORDER BY created_at DESC LIMIT 1`, plantID,
	).Scan(&a.ID, &a.PlantID, &a.PhotoPath, &a.HealthScore, &a.Diagnosis, &a.CareTips, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting latest assessment for plant %d: %w", plantID, err)
	}
	return &a, nil
}

func (r *AssessmentRepo) GetPhotoData(ctx context.Context, id int) ([]byte, string, error) {
	var data []byte
	var mime string
	err := r.db.Pool.QueryRow(ctx,
		`SELECT photo_data, photo_mime FROM assessments WHERE id = $1`, id,
	).Scan(&data, &mime)
	if err != nil {
		return nil, "", fmt.Errorf("getting photo data for assessment %d: %w", id, err)
	}
	return data, mime, nil
}
