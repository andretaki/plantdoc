package handler

import (
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"github.com/andre/plantdoc/internal/gemini"
	"github.com/andre/plantdoc/internal/repository"
)

type Handler struct {
	plants    *repository.PlantRepo
	assess    *repository.AssessmentRepo
	gemini    *gemini.Client
	tmplFS    fs.FS
	uploadDir string
}

func New(plants *repository.PlantRepo, assess *repository.AssessmentRepo, gem *gemini.Client, tmplFS fs.FS, uploadDir string) *Handler {
	return &Handler{
		plants:    plants,
		assess:    assess,
		gemini:    gem,
		tmplFS:    tmplFS,
		uploadDir: uploadDir,
	}
}

func (h *Handler) renderPage(w http.ResponseWriter, page string, data any) {
	funcMap := template.FuncMap{
		"mul": func(a, b int) int { return a * b },
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(h.tmplFS, "layout.html", page)
	if err != nil {
		log.Printf("parsing templates: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Parse partials
	tmpl, err = tmpl.ParseFS(h.tmplFS, "partials/*.html")
	if err != nil {
		log.Printf("parsing partials: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
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
	mux.HandleFunc("GET /photos/{id}", h.ServePhoto)

	// Serve uploaded photos from disk (local dev)
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(h.uploadDir))))
	// Serve static assets
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		http.ServeFile(w, r, "static/style.css")
	})
}

func (h *Handler) ServePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data, mime, err := h.assess.GetPhotoData(r.Context(), id)
	if err != nil || len(data) == 0 {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(data)
}
