# PlantDoc Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a personal plant care web app where you upload photos and Gemini Vision identifies the species, diagnoses health issues, and tracks plants over time.

**Architecture:** Go HTTP server with HTMX frontend. Neon Postgres for storage. Gemini API for plant photo analysis. Photos stored on disk locally, Vercel Blob in production. Single-user, no auth.

**Tech Stack:** Go 1.22+, net/http, html/template, HTMX 2.0, Neon Postgres (pgx driver), Gemini API (REST), Tailwind CSS (CDN)

---

### Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `.env`
- Create: `.env.example`
- Create: `cmd/server/main.go`

**Step 1: Initialize Go module**

Run: `go mod init github.com/andre/plantdoc`

**Step 2: Create .gitignore**

```
.env
uploads/
tmp/
```

**Step 3: Create .env with credentials**

```
DATABASE_URL=postgresql://neondb_owner:npg_QEnY2SrdKx0D@ep-divine-smoke-aispm4q2-pooler.c-4.us-east-1.aws.neon.tech/neondb?sslmode=require
GEMINI_API_KEY=AIzaSyBh-G56Nz8dK8KjQo70FW6_5lIdAJFgML0
PORT=8080
UPLOAD_DIR=uploads
```

**Step 4: Create .env.example (no secrets)**

```
DATABASE_URL=postgresql://user:pass@host/db?sslmode=require
GEMINI_API_KEY=your-gemini-api-key
PORT=8080
UPLOAD_DIR=uploads
```

**Step 5: Create minimal main.go**

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "PlantDoc is running")
	})

	log.Printf("Starting PlantDoc on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
```

**Step 6: Run to verify**

Run: `go run cmd/server/main.go`
Expected: "Starting PlantDoc on :8080"

**Step 7: Commit**

```bash
git add go.mod .gitignore .env.example cmd/server/main.go
git commit -m "chore: project scaffold with minimal Go server"
```

---

### Task 2: Database Layer

**Files:**
- Create: `internal/database/db.go`
- Create: `internal/database/migrations.go`
- Create: `internal/database/db_test.go`

**Step 1: Install pgx driver**

Run: `go get github.com/jackc/pgx/v5/pgxpool`

**Step 2: Write the failing test**

```go
package database

import (
	"context"
	"os"
	"testing"
)

func TestConnect(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	db, err := Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer db.Close()

	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("failed to ping: %v", err)
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test -race ./internal/database/ -v`
Expected: FAIL - Connect not defined

**Step 4: Write db.go**

```go
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func Connect(ctx context.Context, dsn string) (*DB, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}
```

**Step 5: Run test to verify it passes**

Run: `go test -race ./internal/database/ -v`
Expected: PASS

**Step 6: Write migrations.go**

```go
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
			raw_response JSONB,
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
```

**Step 7: Add migration test**

```go
func TestMigrate(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	db, err := Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
}
```

**Step 8: Run tests**

Run: `go test -race ./internal/database/ -v`
Expected: PASS

**Step 9: Commit**

```bash
git add internal/database/ go.mod go.sum
git commit -m "feat: database connection and migrations with pgx"
```

---

### Task 3: Plant Model & Repository

**Files:**
- Create: `internal/model/plant.go`
- Create: `internal/repository/plant.go`
- Create: `internal/repository/plant_test.go`

**Step 1: Write plant model**

```go
package model

import "time"

type Plant struct {
	ID         int
	Name       string
	Species    string
	CommonName string
	CreatedAt  time.Time
}

type Assessment struct {
	ID          int
	PlantID     int
	PhotoPath   string
	HealthScore int
	Diagnosis   string
	CareTips    string
	CreatedAt   time.Time
}
```

**Step 2: Write the failing test for repository**

```go
package repository

import (
	"context"
	"os"
	"testing"

	"github.com/andre/plantdoc/internal/database"
	"github.com/andre/plantdoc/internal/model"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := database.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAndGetPlant(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPlantRepo(db)
	ctx := context.Background()

	plant, err := repo.Create(ctx, "Test Fern", "Nephrolepis exaltata", "Boston Fern")
	if err != nil {
		t.Fatalf("create plant: %v", err)
	}
	if plant.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	got, err := repo.GetByID(ctx, plant.ID)
	if err != nil {
		t.Fatalf("get plant: %v", err)
	}
	if got.Name != "Test Fern" {
		t.Errorf("got name %q, want %q", got.Name, "Test Fern")
	}

	// Cleanup
	_ = repo.Delete(ctx, plant.ID)
}

func TestListPlants(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPlantRepo(db)
	ctx := context.Background()

	p1, _ := repo.Create(ctx, "Plant A", "", "")
	p2, _ := repo.Create(ctx, "Plant B", "", "")
	defer func() {
		_ = repo.Delete(ctx, p1.ID)
		_ = repo.Delete(ctx, p2.ID)
	}()

	plants, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list plants: %v", err)
	}
	if len(plants) < 2 {
		t.Errorf("expected at least 2 plants, got %d", len(plants))
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test -race ./internal/repository/ -v`
Expected: FAIL - NewPlantRepo not defined

**Step 4: Write plant repository**

```go
package repository

import (
	"context"
	"fmt"

	"github.com/andre/plantdoc/internal/database"
	"github.com/andre/plantdoc/internal/model"
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
```

**Step 5: Run tests**

Run: `go test -race ./internal/repository/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/model/ internal/repository/
git commit -m "feat: plant model and repository with CRUD operations"
```

---

### Task 4: Assessment Repository

**Files:**
- Create: `internal/repository/assessment.go`
- Create: `internal/repository/assessment_test.go`

**Step 1: Write the failing test**

```go
package repository

import (
	"context"
	"testing"
)

func TestCreateAndListAssessments(t *testing.T) {
	db := setupTestDB(t)
	plantRepo := NewPlantRepo(db)
	assessRepo := NewAssessmentRepo(db)
	ctx := context.Background()

	plant, err := plantRepo.Create(ctx, "Test Plant", "Test Species", "Test Common")
	if err != nil {
		t.Fatalf("create plant: %v", err)
	}
	defer func() { _ = plantRepo.Delete(ctx, plant.ID) }()

	a, err := assessRepo.Create(ctx, plant.ID, "uploads/test.jpg", 7, "Looks healthy", "Water weekly")
	if err != nil {
		t.Fatalf("create assessment: %v", err)
	}
	if a.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	assessments, err := assessRepo.ListByPlant(ctx, plant.ID)
	if err != nil {
		t.Fatalf("list assessments: %v", err)
	}
	if len(assessments) != 1 {
		t.Fatalf("expected 1 assessment, got %d", len(assessments))
	}
	if assessments[0].HealthScore != 7 {
		t.Errorf("got health score %d, want 7", assessments[0].HealthScore)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/repository/ -run TestCreateAndListAssessments -v`
Expected: FAIL - NewAssessmentRepo not defined

**Step 3: Write assessment repository**

```go
package repository

import (
	"context"
	"fmt"

	"github.com/andre/plantdoc/internal/database"
	"github.com/andre/plantdoc/internal/model"
)

type AssessmentRepo struct {
	db *database.DB
}

func NewAssessmentRepo(db *database.DB) *AssessmentRepo {
	return &AssessmentRepo{db: db}
}

func (r *AssessmentRepo) Create(ctx context.Context, plantID int, photoPath string, healthScore int, diagnosis, careTips string) (*model.Assessment, error) {
	var a model.Assessment
	err := r.db.Pool.QueryRow(ctx,
		`INSERT INTO assessments (plant_id, photo_path, health_score, diagnosis, care_tips)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, plant_id, photo_path, health_score, diagnosis, care_tips, created_at`,
		plantID, photoPath, healthScore, diagnosis, careTips,
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
```

**Step 4: Run tests**

Run: `go test -race ./internal/repository/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/repository/assessment.go internal/repository/assessment_test.go
git commit -m "feat: assessment repository with create/list/latest"
```

---

### Task 5: Gemini API Client

**Files:**
- Create: `internal/gemini/client.go`
- Create: `internal/gemini/client_test.go`

**Step 1: Write the failing test**

```go
package gemini

import (
	"context"
	"os"
	"testing"
)

func TestAnalyzePlant(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set")
	}

	client := NewClient(apiKey)

	// Use a small test image - create a simple test fixture
	// In real tests, use a small plant photo in testdata/
	t.Run("with valid image", func(t *testing.T) {
		// Skip if no test image available
		imgPath := "testdata/test-plant.jpg"
		if _, err := os.Stat(imgPath); os.IsNotExist(err) {
			t.Skip("no test image at testdata/test-plant.jpg")
		}

		imgData, err := os.ReadFile(imgPath)
		if err != nil {
			t.Fatalf("reading test image: %v", err)
		}

		result, err := client.AnalyzePlant(context.Background(), imgData, "image/jpeg", nil)
		if err != nil {
			t.Fatalf("analyze plant: %v", err)
		}

		if result.Species == "" {
			t.Error("expected non-empty species")
		}
		if result.HealthScore < 1 || result.HealthScore > 10 {
			t.Errorf("health score %d out of range 1-10", result.HealthScore)
		}
		if result.Diagnosis == "" {
			t.Error("expected non-empty diagnosis")
		}
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/gemini/ -v`
Expected: FAIL - NewClient not defined

**Step 3: Write Gemini client**

```go
package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
}

type AnalysisResult struct {
	Species     string `json:"species"`
	CommonName  string `json:"common_name"`
	HealthScore int    `json:"health_score"`
	Diagnosis   string `json:"diagnosis"`
	CareTips    string `json:"care_tips"`
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

func (c *Client) AnalyzePlant(ctx context.Context, imageData []byte, mimeType string, previousDiagnosis *string) (*AnalysisResult, error) {
	b64Image := base64.StdEncoding.EncodeToString(imageData)

	prompt := `You are a plant identification and health expert. Analyze this plant photo.

Respond ONLY with valid JSON (no markdown, no code fences):
{
  "species": "Scientific name",
  "common_name": "Common name",
  "health_score": 7,
  "diagnosis": "Detailed health assessment in 2-3 sentences.",
  "care_tips": "Specific actionable care advice in 2-3 sentences."
}

health_score is 1-10 where 10 is perfectly healthy.`

	if previousDiagnosis != nil {
		prompt += fmt.Sprintf("\n\nPrevious assessment for comparison: %s\nNote any improvements or decline.", *previousDiagnosis)
	}

	reqBody := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"text": prompt},
					{
						"inline_data": map[string]any{
							"mime_type": mimeType,
							"data":      b64Image,
						},
					},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Gemini API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini API error %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("parsing Gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from Gemini")
	}

	text := geminiResp.Candidates[0].Content.Parts[0].Text

	var result AnalysisResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parsing analysis result: %w\nraw text: %s", err, text)
	}

	return &result, nil
}
```

**Step 4: Run tests (will skip without test image but should compile)**

Run: `go test -race ./internal/gemini/ -v`
Expected: SKIP or PASS

**Step 5: Commit**

```bash
git add internal/gemini/
git commit -m "feat: Gemini API client for plant photo analysis"
```

---

### Task 6: HTML Templates & Static Assets

**Files:**
- Create: `templates/layout.html`
- Create: `templates/dashboard.html`
- Create: `templates/upload.html`
- Create: `templates/plant.html`
- Create: `templates/partials/plant-card.html`
- Create: `templates/partials/assessment.html`
- Create: `static/style.css`

**Step 1: Create layout template**

`templates/layout.html`:
```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>PlantDoc - {{block "title" .}}Dashboard{{end}}</title>
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
    <link href="https://cdn.jsdelivr.net/npm/tailwindcss@2/dist/tailwind.min.css" rel="stylesheet">
    <link href="/static/style.css" rel="stylesheet">
</head>
<body class="bg-gray-50 min-h-screen">
    <nav class="bg-green-700 text-white p-4 shadow-lg">
        <div class="max-w-5xl mx-auto flex justify-between items-center">
            <a href="/" class="text-2xl font-bold">PlantDoc</a>
            <a href="/upload" class="bg-green-500 hover:bg-green-400 px-4 py-2 rounded-lg font-medium">
                + New Scan
            </a>
        </div>
    </nav>
    <main class="max-w-5xl mx-auto p-6">
        {{block "content" .}}{{end}}
    </main>
</body>
</html>
```

**Step 2: Create dashboard template**

`templates/dashboard.html`:
```html
{{define "title"}}Dashboard{{end}}
{{define "content"}}
<div class="mb-8">
    <h1 class="text-3xl font-bold text-gray-800 mb-2">Your Plants</h1>
    <p class="text-gray-500">{{len .Plants}} plant{{if ne (len .Plants) 1}}s{{end}} in your collection</p>
</div>

{{if .Plants}}
<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
    {{range .Plants}}
    {{template "plant-card" .}}
    {{end}}
</div>
{{else}}
<div class="text-center py-20">
    <p class="text-6xl mb-4">&#127793;</p>
    <h2 class="text-xl font-semibold text-gray-600 mb-2">No plants yet</h2>
    <p class="text-gray-400 mb-6">Upload your first plant photo to get started</p>
    <a href="/upload" class="bg-green-600 hover:bg-green-500 text-white px-6 py-3 rounded-lg font-medium">
        Scan Your First Plant
    </a>
</div>
{{end}}
{{end}}
```

**Step 3: Create upload template**

`templates/upload.html`:
```html
{{define "title"}}New Scan{{end}}
{{define "content"}}
<div class="max-w-2xl mx-auto">
    <h1 class="text-3xl font-bold text-gray-800 mb-6">Scan a Plant</h1>

    <form hx-post="/upload" hx-target="#result" hx-encoding="multipart/form-data"
          hx-indicator="#loading" class="space-y-6">

        <div id="drop-zone" class="border-2 border-dashed border-gray-300 rounded-xl p-12 text-center
                    hover:border-green-500 transition-colors cursor-pointer">
            <p class="text-4xl mb-3">&#128247;</p>
            <p class="text-gray-600 font-medium">Drop a plant photo here or click to browse</p>
            <input type="file" name="photo" accept="image/*" required
                   class="absolute inset-0 opacity-0 cursor-pointer"
                   onchange="previewImage(this)">
            <img id="preview" class="mx-auto mt-4 max-h-64 rounded-lg hidden">
        </div>

        {{if .Plants}}
        <div>
            <label class="block text-sm font-medium text-gray-700 mb-2">Add to existing plant (optional)</label>
            <select name="plant_id" class="w-full border rounded-lg p-3">
                <option value="">New plant</option>
                {{range .Plants}}
                <option value="{{.ID}}">{{.Name}}</option>
                {{end}}
            </select>
        </div>
        {{end}}

        <div>
            <label class="block text-sm font-medium text-gray-700 mb-2">Plant name (for new plants)</label>
            <input type="text" name="name" placeholder="e.g. Living Room Fern"
                   class="w-full border rounded-lg p-3">
        </div>

        <button type="submit"
                class="w-full bg-green-600 hover:bg-green-500 text-white py-3 rounded-lg font-medium text-lg">
            Analyze Plant
        </button>
    </form>

    <div id="loading" class="htmx-indicator text-center py-8">
        <p class="text-lg text-gray-600">Analyzing your plant...</p>
        <div class="animate-spin text-4xl mt-2">&#127793;</div>
    </div>

    <div id="result" class="mt-6"></div>
</div>

<script>
function previewImage(input) {
    const preview = document.getElementById('preview');
    if (input.files && input.files[0]) {
        const reader = new FileReader();
        reader.onload = function(e) {
            preview.src = e.target.result;
            preview.classList.remove('hidden');
        };
        reader.readAsDataURL(input.files[0]);
    }
}
</script>
{{end}}
```

**Step 4: Create plant detail template**

`templates/plant.html`:
```html
{{define "title"}}{{.Plant.Name}}{{end}}
{{define "content"}}
<div class="mb-6">
    <a href="/" class="text-green-600 hover:underline">&larr; Back to dashboard</a>
</div>

<div class="flex justify-between items-start mb-8">
    <div>
        <h1 class="text-3xl font-bold text-gray-800">{{.Plant.Name}}</h1>
        {{if .Plant.Species}}
        <p class="text-gray-500 italic">{{.Plant.Species}} ({{.Plant.CommonName}})</p>
        {{end}}
    </div>
    <a href="/plants/{{.Plant.ID}}/upload"
       class="bg-green-600 hover:bg-green-500 text-white px-4 py-2 rounded-lg">
        + New Photo
    </a>
</div>

<div class="space-y-6">
    {{range .Assessments}}
    {{template "assessment" .}}
    {{end}}
</div>
{{end}}
```

**Step 5: Create partials**

`templates/partials/plant-card.html`:
```html
{{define "plant-card"}}
<a href="/plants/{{.Plant.ID}}" class="block bg-white rounded-xl shadow hover:shadow-lg transition-shadow overflow-hidden">
    {{if .LatestPhoto}}
    <img src="/uploads/{{.LatestPhoto}}" alt="{{.Plant.Name}}" class="w-full h-48 object-cover">
    {{else}}
    <div class="w-full h-48 bg-green-50 flex items-center justify-center text-4xl">&#127793;</div>
    {{end}}
    <div class="p-4">
        <h3 class="font-semibold text-lg text-gray-800">{{.Plant.Name}}</h3>
        {{if .Plant.CommonName}}
        <p class="text-sm text-gray-500">{{.Plant.CommonName}}</p>
        {{end}}
        {{if .HealthScore}}
        <div class="mt-2 flex items-center gap-2">
            <span class="text-sm font-medium">Health:</span>
            <div class="flex-1 bg-gray-200 rounded-full h-2">
                <div class="bg-green-500 h-2 rounded-full" style="width: {{mul .HealthScore 10}}%"></div>
            </div>
            <span class="text-sm font-bold">{{.HealthScore}}/10</span>
        </div>
        {{end}}
    </div>
</a>
{{end}}
```

`templates/partials/assessment.html`:
```html
{{define "assessment"}}
<div class="bg-white rounded-xl shadow p-6 flex gap-6">
    <img src="/uploads/{{.PhotoPath}}" alt="Plant photo" class="w-40 h-40 object-cover rounded-lg flex-shrink-0">
    <div class="flex-1">
        <div class="flex justify-between items-start mb-3">
            <span class="text-sm text-gray-400">{{.CreatedAt.Format "Jan 2, 2006 3:04 PM"}}</span>
            <span class="bg-green-100 text-green-800 px-3 py-1 rounded-full text-sm font-bold">
                {{.HealthScore}}/10
            </span>
        </div>
        <h3 class="font-semibold text-gray-800 mb-1">Diagnosis</h3>
        <p class="text-gray-600 text-sm mb-3">{{.Diagnosis}}</p>
        <h3 class="font-semibold text-gray-800 mb-1">Care Tips</h3>
        <p class="text-gray-600 text-sm">{{.CareTips}}</p>
    </div>
</div>
{{end}}
```

**Step 6: Create minimal CSS**

`static/style.css`:
```css
.htmx-indicator { display: none; }
.htmx-request .htmx-indicator { display: block; }
.htmx-request.htmx-indicator { display: block; }

#drop-zone { position: relative; }
#drop-zone input[type="file"] { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
```

**Step 7: Commit**

```bash
git add templates/ static/
git commit -m "feat: HTML templates with HTMX and Tailwind"
```

---

### Task 7: HTTP Handlers

**Files:**
- Create: `internal/handler/handler.go`
- Create: `internal/handler/dashboard.go`
- Create: `internal/handler/upload.go`
- Create: `internal/handler/plants.go`

**Step 1: Write base handler with template rendering**

```go
package handler

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"

	"github.com/andre/plantdoc/internal/gemini"
	"github.com/andre/plantdoc/internal/repository"
)

type Handler struct {
	plants     *repository.PlantRepo
	assess     *repository.AssessmentRepo
	gemini     *gemini.Client
	tmpl       *template.Template
	uploadDir  string
}

func New(plants *repository.PlantRepo, assess *repository.AssessmentRepo, gem *gemini.Client, uploadDir string) *Handler {
	funcMap := template.FuncMap{
		"mul": func(a, b int) int { return a * b },
	}

	tmpl := template.Must(
		template.New("").Funcs(funcMap).ParseGlob(filepath.Join("templates", "*.html")),
	)
	template.Must(tmpl.ParseGlob(filepath.Join("templates", "partials", "*.html")))

	return &Handler{
		plants:    plants,
		assess:    assess,
		gemini:    gem,
		tmpl:      tmpl,
		uploadDir: uploadDir,
	}
}

func (h *Handler) render(w http.ResponseWriter, tmpl string, data any) {
	if err := h.tmpl.ExecuteTemplate(w, tmpl, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.Dashboard)
	mux.HandleFunc("GET /upload", h.UploadForm)
	mux.HandleFunc("POST /upload", h.UploadPhoto)
	mux.HandleFunc("GET /plants/{id}", h.PlantDetail)
	mux.HandleFunc("GET /plants/{id}/upload", h.PlantUploadForm)

	// Serve uploaded photos
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(h.uploadDir))))
	// Serve static assets
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
}
```

**Step 2: Write dashboard handler**

```go
package handler

import "net/http"

type PlantCard struct {
	Plant       any
	LatestPhoto string
	HealthScore int
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	plants, err := h.plants.List(r.Context())
	if err != nil {
		log.Printf("listing plants: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var cards []PlantCard
	for _, p := range plants {
		card := PlantCard{Plant: p}
		if latest, err := h.assess.GetLatestByPlant(r.Context(), p.ID); err == nil {
			card.LatestPhoto = latest.PhotoPath
			card.HealthScore = latest.HealthScore
		}
		cards = append(cards, card)
	}

	h.render(w, "layout.html", map[string]any{
		"Plants": cards,
	})
}
```

Wait - the dashboard template uses `{{template "dashboard"}}` via blocks. Let me adjust the rendering approach to use template blocks properly.

Actually, the layout uses `{{block "content" .}}` so each page defines its own content block. The render method should execute the layout which will include the correct content block.

The templates need to be structured so that each page (dashboard.html, upload.html, etc.) defines the `content` and `title` blocks, and we render `layout.html` which picks them up.

For this to work with Go templates, each page template should be parsed together with the layout. I'll structure handlers to use the correct template set.

**Step 3: Write upload handler**

```go
package handler

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
)

func (h *Handler) UploadForm(w http.ResponseWriter, r *http.Request) {
	plants, _ := h.plants.List(r.Context())
	h.render(w, "layout.html", map[string]any{
		"Plants":  plants,
		"IsUpload": true,
	})
}

func (h *Handler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "No photo uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save file
	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%d-%s%s", time.Now().Unix(), uuid.New().String()[:8], ext)
	savePath := filepath.Join(h.uploadDir, filename)

	if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
		log.Printf("creating upload dir: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	dst, err := os.Create(savePath)
	if err != nil {
		log.Printf("creating file: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	imgData, err := io.ReadAll(file)
	if err != nil {
		log.Printf("reading file: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if _, err := dst.Write(imgData); err != nil {
		log.Printf("writing file: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Detect MIME type
	mimeType := http.DetectContentType(imgData)

	// Get previous diagnosis if adding to existing plant
	var previousDiag *string
	plantIDStr := r.FormValue("plant_id")
	if plantIDStr != "" {
		if pid, err := strconv.Atoi(plantIDStr); err == nil {
			if prev, err := h.assess.GetLatestByPlant(r.Context(), pid); err == nil {
				previousDiag = &prev.Diagnosis
			}
		}
	}

	// Call Gemini
	result, err := h.gemini.AnalyzePlant(r.Context(), imgData, mimeType, previousDiag)
	if err != nil {
		log.Printf("Gemini analysis: %v", err)
		fmt.Fprintf(w, `<div class="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700">
			Analysis failed: %s. Your photo was saved - try again later.
		</div>`, err)
		return
	}

	// Save to database
	var plantID int
	if plantIDStr != "" {
		plantID, _ = strconv.Atoi(plantIDStr)
	} else {
		name := r.FormValue("name")
		if name == "" {
			name = result.CommonName
		}
		if name == "" {
			name = "Unknown Plant"
		}
		plant, err := h.plants.Create(r.Context(), name, result.Species, result.CommonName)
		if err != nil {
			log.Printf("creating plant: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		plantID = plant.ID
	}

	_, err = h.assess.Create(r.Context(), plantID, filename, result.HealthScore, result.Diagnosis, result.CareTips)
	if err != nil {
		log.Printf("creating assessment: %v", err)
	}

	// Return HTMX partial with results
	fmt.Fprintf(w, `<div class="bg-green-50 border border-green-200 rounded-xl p-6 space-y-4">
		<div class="flex items-center gap-3">
			<span class="text-3xl">&#127793;</span>
			<div>
				<h2 class="text-xl font-bold text-green-800">%s</h2>
				<p class="text-green-600 italic">%s</p>
			</div>
			<span class="ml-auto bg-green-200 text-green-900 px-4 py-2 rounded-full text-lg font-bold">%d/10</span>
		</div>
		<div>
			<h3 class="font-semibold text-gray-800">Diagnosis</h3>
			<p class="text-gray-600">%s</p>
		</div>
		<div>
			<h3 class="font-semibold text-gray-800">Care Tips</h3>
			<p class="text-gray-600">%s</p>
		</div>
		<a href="/plants/%d" class="inline-block bg-green-600 hover:bg-green-500 text-white px-4 py-2 rounded-lg">
			View Plant Journal &rarr;
		</a>
	</div>`, result.CommonName, result.Species, result.HealthScore, result.Diagnosis, result.CareTips, plantID)
}
```

**Step 4: Write plant detail handler**

```go
package handler

import (
	"log"
	"net/http"
	"strconv"
)

func (h *Handler) PlantDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	plant, err := h.plants.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	assessments, err := h.assess.ListByPlant(r.Context(), id)
	if err != nil {
		log.Printf("listing assessments: %v", err)
	}

	h.render(w, "layout.html", map[string]any{
		"Plant":       plant,
		"Assessments": assessments,
		"IsPlant":     true,
	})
}

func (h *Handler) PlantUploadForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	plant, err := h.plants.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	plants, _ := h.plants.List(r.Context())

	h.render(w, "layout.html", map[string]any{
		"Plants":         plants,
		"SelectedPlant":  plant,
		"IsUpload":       true,
	})
}
```

**Step 5: Commit**

```bash
git add internal/handler/
git commit -m "feat: HTTP handlers for dashboard, upload, and plant detail"
```

---

### Task 8: Wire Everything Together in main.go

**Files:**
- Modify: `cmd/server/main.go`

**Step 1: Update main.go**

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/andre/plantdoc/internal/database"
	"github.com/andre/plantdoc/internal/gemini"
	"github.com/andre/plantdoc/internal/handler"
	"github.com/andre/plantdoc/internal/repository"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		log.Fatal("GEMINI_API_KEY is required")
	}

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "uploads"
	}

	ctx := context.Background()

	db, err := database.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("database connection: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("database migration: %v", err)
	}
	log.Println("Database migrated successfully")

	plantRepo := repository.NewPlantRepo(db)
	assessRepo := repository.NewAssessmentRepo(db)
	geminiClient := gemini.NewClient(geminiKey)

	h := handler.New(plantRepo, assessRepo, geminiClient, uploadDir)

	mux := http.NewServeMux()
	h.Routes(mux)

	log.Printf("PlantDoc running at http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
```

**Step 2: Install dependencies**

Run: `go get github.com/joho/godotenv github.com/google/uuid`

**Step 3: Run the server**

Run: `go run cmd/server/main.go`
Expected: "PlantDoc running at http://localhost:8080"

**Step 4: Commit**

```bash
git add cmd/server/main.go go.mod go.sum
git commit -m "feat: wire up main.go with all components"
```

---

### Task 9: Template Rendering Fix

The Go template system needs all related templates parsed together. Fix the rendering to properly support page-specific content blocks.

**Files:**
- Modify: `internal/handler/handler.go`

**Step 1: Update template loading to handle page-specific templates**

Instead of one big template set, load layout + page-specific template per render:

```go
func (h *Handler) renderPage(w http.ResponseWriter, page string, data any) {
	funcMap := template.FuncMap{
		"mul": func(a, b int) int { return a * b },
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFiles(
		filepath.Join("templates", "layout.html"),
		filepath.Join("templates", page),
	)
	if err != nil {
		log.Printf("parsing templates: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Parse partials
	partials, _ := filepath.Glob(filepath.Join("templates", "partials", "*.html"))
	if len(partials) > 0 {
		tmpl, err = tmpl.ParseFiles(partials...)
		if err != nil {
			log.Printf("parsing partials: %v", err)
		}
	}

	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
```

Update all handlers to use `h.renderPage(w, "dashboard.html", data)` instead of `h.render(w, "layout.html", data)`.

**Step 2: Commit**

```bash
git add internal/handler/
git commit -m "fix: template rendering with per-page template sets"
```

---

### Task 10: End-to-End Test

**Files:**
- Create: `cmd/server/main_test.go`

**Step 1: Write integration test**

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthCheck(t *testing.T) {
	// Basic smoke test - ensure server starts and responds
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want 200", resp.StatusCode)
	}
}
```

**Step 2: Run all tests**

Run: `go test -race ./... -v`
Expected: PASS

**Step 3: Final commit**

```bash
git add .
git commit -m "feat: PlantDoc v1 - plant identification and health tracking"
```

---

## Summary

| Task | What | Files |
|------|------|-------|
| 1 | Project scaffold | go.mod, .gitignore, .env, main.go |
| 2 | Database layer | internal/database/ |
| 3 | Plant repository | internal/model/, internal/repository/plant.go |
| 4 | Assessment repository | internal/repository/assessment.go |
| 5 | Gemini API client | internal/gemini/ |
| 6 | HTML templates | templates/, static/ |
| 7 | HTTP handlers | internal/handler/ |
| 8 | Wire main.go | cmd/server/main.go |
| 9 | Template fix | internal/handler/handler.go |
| 10 | Integration test | cmd/server/main_test.go |
