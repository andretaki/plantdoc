package handler

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/andre/plantdoc/pkg/database"
	"github.com/andre/plantdoc/pkg/gemini"
	apphandler "github.com/andre/plantdoc/pkg/handler"
	"github.com/andre/plantdoc/pkg/repository"
	"github.com/andre/plantdoc/templates"
)

var (
	mux  *http.ServeMux
	once sync.Once
)

func setup() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		log.Fatal("GEMINI_API_KEY is required")
	}

	ctx := context.Background()

	db, err := database.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("database connection: %v", err)
	}

	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("database migration: %v", err)
	}

	plantRepo := repository.NewPlantRepo(db)
	assessRepo := repository.NewAssessmentRepo(db)
	geminiClient := gemini.NewClient(geminiKey)

	h := apphandler.New(plantRepo, assessRepo, geminiClient, templates.FS(), "/tmp/uploads")

	mux = http.NewServeMux()
	h.Routes(mux)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(setup)
	mux.ServeHTTP(w, r)
}
