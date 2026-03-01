package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/andre/plantdoc/pkg/database"
	"github.com/andre/plantdoc/pkg/gemini"
	"github.com/andre/plantdoc/pkg/handler"
	"github.com/andre/plantdoc/pkg/repository"
	"github.com/andre/plantdoc/templates"
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
	profileRepo := repository.NewProfileRepo(db)
	assessRepo := repository.NewAssessmentRepo(db)
	geminiClient := gemini.NewClient(geminiKey)

	h := handler.New(plantRepo, profileRepo, assessRepo, geminiClient, templates.FS(), uploadDir)

	mux := http.NewServeMux()
	h.Routes(mux)

	log.Printf("PlantDoc running at http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
