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

	"github.com/andre/plantdoc/pkg/gemini"
	"github.com/andre/plantdoc/pkg/model"
	"github.com/google/uuid"
)

func (h *Handler) UploadForm(w http.ResponseWriter, r *http.Request) {
	plants, _ := h.plants.List(r.Context())

	var cards []PlantCard
	for _, p := range plants {
		card := PlantCard{Plant: p}
		if latest, err := h.assess.GetLatestByPlant(r.Context(), p.ID); err == nil {
			card.LatestPhoto = latest.PhotoPath
			card.LatestPhotoID = latest.ID
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
			card.LatestPhotoID = latest.ID
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

	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%d-%s%s", time.Now().Unix(), uuid.New().String()[:8], ext)
	_ = os.MkdirAll(h.uploadDir, 0755)
	_ = os.WriteFile(filepath.Join(h.uploadDir, filename), imgData, 0644)

	mimeType := http.DetectContentType(imgData)
	scanMode := r.FormValue("scan_mode")

	if scanMode == "bulk" {
		h.handleBulkScan(w, r, imgData, mimeType, filename)
		return
	}

	// Single plant scan
	var previousDiag *string
	plantIDStr := r.FormValue("plant_id")
	if plantIDStr != "" {
		if pid, err := strconv.Atoi(plantIDStr); err == nil {
			if prev, err := h.assess.GetLatestByPlant(r.Context(), pid); err == nil {
				previousDiag = &prev.Diagnosis
			}
		}
	}

	result, err := h.gemini.AnalyzePlant(r.Context(), imgData, mimeType, previousDiag)
	if err != nil {
		log.Printf("Gemini analysis: %v", err)
		fmt.Fprintf(w, `<div class="error-card">Analysis failed: %s</div>`, html.EscapeString(err.Error()))
		return
	}

	plantID, err := h.savePlantResult(r, plantIDStr, result, filename, imgData, mimeType)
	if err != nil {
		log.Printf("saving result: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.renderResultCard(w, result, plantID)
}

func (h *Handler) handleBulkScan(w http.ResponseWriter, r *http.Request, imgData []byte, mimeType, filename string) {
	bulk, err := h.gemini.AnalyzeBulk(r.Context(), imgData, mimeType)
	if err != nil {
		log.Printf("Gemini bulk analysis: %v", err)
		fmt.Fprintf(w, `<div class="error-card">Bulk analysis failed: %s</div>`, html.EscapeString(err.Error()))
		return
	}

	fmt.Fprintf(w, `<div class="bulk-results"><div class="bulk-header"><span class="bulk-count">%d</span> plants detected</div>`, bulk.PlantCount)

	for i, result := range bulk.Plants {
		plantID, err := h.savePlantResult(r, "", &result, fmt.Sprintf("%d-%s", i, filename), imgData, mimeType)
		if err != nil {
			log.Printf("saving bulk plant %d: %v", i, err)
			continue
		}
		h.renderResultCard(w, &result, plantID)
	}

	fmt.Fprintf(w, `</div>`)
}

func (h *Handler) savePlantResult(r *http.Request, plantIDStr string, result *gemini.AnalysisResult, filename string, imgData []byte, mimeType string) (int, error) {
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
			return 0, fmt.Errorf("creating plant: %w", err)
		}
		plantID = plant.ID
	}

	assess := &model.Assessment{
		HealthScore:    result.HealthScore,
		Confidence:     result.Confidence,
		Diagnosis:      result.Diagnosis,
		CareTips:       result.CareTips,
		Foliage:        result.SubScores.Foliage,
		Hydration:      result.SubScores.Hydration,
		PestRisk:       result.SubScores.PestRisk,
		Vitality:       result.SubScores.Vitality,
		Urgent:         result.Urgent,
		SeasonalAdvice: result.SeasonalAdvice,
	}

	_, err := h.assess.Create(r.Context(), plantID, filename, imgData, mimeType, assess)
	if err != nil {
		log.Printf("creating assessment: %v", err)
	}

	return plantID, nil
}

func (h *Handler) renderResultCard(w http.ResponseWriter, result *gemini.AnalysisResult, plantID int) {
	urgentHTML := ""
	if result.Urgent != "" {
		urgentHTML = fmt.Sprintf(`<div class="result-urgent"><div class="result-section-title">Urgent</div><p>%s</p></div>`, html.EscapeString(result.Urgent))
	}

	seasonalHTML := ""
	if result.SeasonalAdvice != "" {
		seasonalHTML = fmt.Sprintf(`<div class="result-section"><div class="result-section-title">Seasonal</div><p>%s</p></div>`, html.EscapeString(result.SeasonalAdvice))
	}

	confidenceLabel := result.Confidence
	if confidenceLabel == "" {
		confidenceLabel = "medium"
	}

	fmt.Fprintf(w, `<div class="result-card">
		<div class="result-header">
			<div class="result-names">
				<div class="result-common">%s</div>
				<div class="result-species">%s <span class="result-confidence confidence-%s">%s confidence</span></div>
			</div>
			<div class="result-score">%d/10</div>
		</div>
		<div class="result-subscores">
			<div class="subscore"><span class="subscore-label">Foliage</span><span class="subscore-value">%d</span></div>
			<div class="subscore"><span class="subscore-label">Hydration</span><span class="subscore-value">%d</span></div>
			<div class="subscore"><span class="subscore-label">Pest Risk</span><span class="subscore-value">%d</span></div>
			<div class="subscore"><span class="subscore-label">Vitality</span><span class="subscore-value">%d</span></div>
		</div>
		%s
		<div class="result-section">
			<div class="result-section-title">Diagnosis</div>
			<p>%s</p>
		</div>
		<div class="result-section">
			<div class="result-section-title">Care Plan</div>
			<p>%s</p>
		</div>
		%s
		<div class="result-action">
			<a href="/plants/%d" class="btn-primary">View Plant Journal &rarr;</a>
		</div>
	</div>`,
		html.EscapeString(result.CommonName),
		html.EscapeString(result.Species),
		html.EscapeString(confidenceLabel),
		html.EscapeString(confidenceLabel),
		result.HealthScore,
		result.SubScores.Foliage,
		result.SubScores.Hydration,
		result.SubScores.PestRisk,
		result.SubScores.Vitality,
		urgentHTML,
		html.EscapeString(result.Diagnosis),
		html.EscapeString(result.CareTips),
		seasonalHTML,
		plantID)
}
