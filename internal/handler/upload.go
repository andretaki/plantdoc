package handler

import (
	"fmt"
	"html"
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
	h.renderPage(w, "upload.html", map[string]any{
		"Plants": plants,
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

	h.renderPage(w, "upload.html", map[string]any{
		"Plants":        plants,
		"SelectedPlant": plant,
	})
}

func (h *Handler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "File too large (max 10MB)", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "No photo uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read image data
	imgData, err := io.ReadAll(file)
	if err != nil {
		log.Printf("reading file: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Save file to disk
	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%d-%s%s", time.Now().Unix(), uuid.New().String()[:8], ext)
	savePath := filepath.Join(h.uploadDir, filename)

	if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
		log.Printf("creating upload dir: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(savePath, imgData, 0644); err != nil {
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

	// Call Gemini for analysis
	result, err := h.gemini.AnalyzePlant(r.Context(), imgData, mimeType, previousDiag)
	if err != nil {
		log.Printf("Gemini analysis: %v", err)
		fmt.Fprintf(w, `<div class="error-card">Analysis failed: %s. Your photo was saved — try again later.</div>`, html.EscapeString(err.Error()))
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
	fmt.Fprintf(w, `<div class="result-card">
		<div class="result-header">
			<div class="result-names">
				<div class="result-common">%s</div>
				<div class="result-species">%s</div>
			</div>
			<div class="result-score">%d/10</div>
		</div>
		<div class="result-section">
			<div class="result-section-title">Diagnosis</div>
			<p>%s</p>
		</div>
		<div class="result-section">
			<div class="result-section-title">Care Notes</div>
			<p>%s</p>
		</div>
		<div class="result-action">
			<a href="/plants/%d" class="btn-primary">View Plant Journal &rarr;</a>
		</div>
	</div>`,
		html.EscapeString(result.CommonName),
		html.EscapeString(result.Species),
		result.HealthScore,
		html.EscapeString(result.Diagnosis),
		html.EscapeString(result.CareTips),
		plantID)
}
