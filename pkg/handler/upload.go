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

	var cards []PlantCard
	for _, p := range plants {
		card := PlantCard{Plant: p}
		if latest, err := h.assess.GetLatestByPlant(r.Context(), p.ID); err == nil {
			card.LatestPhoto = latest.PhotoPath
			card.HealthScore = latest.HealthScore
		}
		cards = append(cards, card)
	}

	h.renderPage(w, "upload.html", map[string]any{
		"Plants": cards,
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
	var cards []PlantCard
	for _, p := range plants {
		card := PlantCard{Plant: p}
		if latest, err := h.assess.GetLatestByPlant(r.Context(), p.ID); err == nil {
			card.LatestPhoto = latest.PhotoPath
			card.HealthScore = latest.HealthScore
		}
		cards = append(cards, card)
	}

	h.renderPage(w, "upload.html", map[string]any{
		"Plants":        cards,
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

	imgData, err := io.ReadAll(file)
	if err != nil {
		log.Printf("reading file: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Save file to disk (local dev)
	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%d-%s%s", time.Now().Unix(), uuid.New().String()[:8], ext)

	_ = os.MkdirAll(h.uploadDir, 0755)
	_ = os.WriteFile(filepath.Join(h.uploadDir, filename), imgData, 0644)

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

	// Store photo data in DB (for Vercel/serverless) and on disk (for local)
	assess, err := h.assess.Create(r.Context(), plantID, filename, imgData, mimeType, result.HealthScore, result.Diagnosis, result.CareTips)
	if err != nil {
		log.Printf("creating assessment: %v", err)
	}

	// Photo URL: use /photos/{id} for DB-backed serving
	photoURL := fmt.Sprintf("/uploads/%s", filename)
	if assess != nil {
		photoURL = fmt.Sprintf("/photos/%d", assess.ID)
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

	_ = photoURL // used in templates via /photos/{id}
}
