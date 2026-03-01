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

func (r *AssessmentRepo) Create(ctx context.Context, plantID int, photoPath string, photoData []byte, photoMime string, a *model.Assessment) (*model.Assessment, error) {
	var out model.Assessment
	err := r.db.Pool.QueryRow(ctx,
		`INSERT INTO assessments (plant_id, photo_path, photo_data, photo_mime, health_score, confidence, diagnosis, care_tips, foliage, hydration, pest_risk, vitality, urgent, seasonal_advice)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 RETURNING id, plant_id, photo_path, health_score, confidence, diagnosis, care_tips, foliage, hydration, pest_risk, vitality, urgent, seasonal_advice, created_at`,
		plantID, photoPath, photoData, photoMime,
		a.HealthScore, a.Confidence, a.Diagnosis, a.CareTips,
		a.Foliage, a.Hydration, a.PestRisk, a.Vitality,
		a.Urgent, a.SeasonalAdvice,
	).Scan(&out.ID, &out.PlantID, &out.PhotoPath, &out.HealthScore, &out.Confidence,
		&out.Diagnosis, &out.CareTips, &out.Foliage, &out.Hydration, &out.PestRisk,
		&out.Vitality, &out.Urgent, &out.SeasonalAdvice, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating assessment: %w", err)
	}
	return &out, nil
}

func (r *AssessmentRepo) ListByPlant(ctx context.Context, plantID int) ([]model.Assessment, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, plant_id, photo_path, health_score,
		        COALESCE(confidence, ''), COALESCE(diagnosis, ''), COALESCE(care_tips, ''),
		        COALESCE(foliage, 0), COALESCE(hydration, 0), COALESCE(pest_risk, 0), COALESCE(vitality, 0),
		        COALESCE(urgent, ''), COALESCE(seasonal_advice, ''), created_at
		 FROM assessments WHERE plant_id = $1 ORDER BY created_at DESC`, plantID)
	if err != nil {
		return nil, fmt.Errorf("listing assessments: %w", err)
	}
	defer rows.Close()

	var assessments []model.Assessment
	for rows.Next() {
		var a model.Assessment
		if err := rows.Scan(&a.ID, &a.PlantID, &a.PhotoPath, &a.HealthScore,
			&a.Confidence, &a.Diagnosis, &a.CareTips,
			&a.Foliage, &a.Hydration, &a.PestRisk, &a.Vitality,
			&a.Urgent, &a.SeasonalAdvice, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning assessment: %w", err)
		}
		assessments = append(assessments, a)
	}
	return assessments, nil
}

func (r *AssessmentRepo) GetLatestByPlant(ctx context.Context, plantID int) (*model.Assessment, error) {
	var a model.Assessment
	err := r.db.Pool.QueryRow(ctx,
		`SELECT id, plant_id, photo_path, health_score,
		        COALESCE(confidence, ''), COALESCE(diagnosis, ''), COALESCE(care_tips, ''),
		        COALESCE(foliage, 0), COALESCE(hydration, 0), COALESCE(pest_risk, 0), COALESCE(vitality, 0),
		        COALESCE(urgent, ''), COALESCE(seasonal_advice, ''), created_at
		 FROM assessments WHERE plant_id = $1 ORDER BY created_at DESC LIMIT 1`, plantID,
	).Scan(&a.ID, &a.PlantID, &a.PhotoPath, &a.HealthScore,
		&a.Confidence, &a.Diagnosis, &a.CareTips,
		&a.Foliage, &a.Hydration, &a.PestRisk, &a.Vitality,
		&a.Urgent, &a.SeasonalAdvice, &a.CreatedAt)
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

func (r *AssessmentRepo) GetPhotoDataByProfile(ctx context.Context, assessmentID, profileID int) ([]byte, string, error) {
	var data []byte
	var mime string
	err := r.db.Pool.QueryRow(ctx,
		`SELECT a.photo_data, a.photo_mime
		 FROM assessments a
		 JOIN plants p ON p.id = a.plant_id
		 WHERE a.id = $1 AND p.profile_id = $2`,
		assessmentID, profileID,
	).Scan(&data, &mime)
	if err != nil {
		return nil, "", fmt.Errorf("getting photo data for assessment %d in profile %d: %w", assessmentID, profileID, err)
	}
	return data, mime, nil
}
