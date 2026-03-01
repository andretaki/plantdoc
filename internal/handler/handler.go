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
	plants    *repository.PlantRepo
	assess    *repository.AssessmentRepo
	gemini    *gemini.Client
	tmplDir   string
	uploadDir string
}

func New(plants *repository.PlantRepo, assess *repository.AssessmentRepo, gem *gemini.Client, uploadDir string) *Handler {
	return &Handler{
		plants:    plants,
		assess:    assess,
		gemini:    gem,
		tmplDir:   "templates",
		uploadDir: uploadDir,
	}
}

func (h *Handler) renderPage(w http.ResponseWriter, page string, data any) {
	funcMap := template.FuncMap{
		"mul": func(a, b int) int { return a * b },
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFiles(
		filepath.Join(h.tmplDir, "layout.html"),
		filepath.Join(h.tmplDir, page),
	)
	if err != nil {
		log.Printf("parsing templates: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	partials, _ := filepath.Glob(filepath.Join(h.tmplDir, "partials", "*.html"))
	if len(partials) > 0 {
		tmpl, err = tmpl.ParseFiles(partials...)
		if err != nil {
			log.Printf("parsing partials: %v", err)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("executing template: %v", err)
	}
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.Dashboard)
	mux.HandleFunc("GET /upload", h.UploadForm)
	mux.HandleFunc("POST /upload", h.UploadPhoto)
	mux.HandleFunc("GET /plants/{id}", h.PlantDetail)
	mux.HandleFunc("GET /plants/{id}/upload", h.PlantUploadForm)

	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(h.uploadDir))))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
}
