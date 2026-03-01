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
	"strings"
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
		writeErrorCard(w, "File too large (max 10MB).")
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		writeErrorCard(w, "No photo uploaded.")
		return
	}
	defer file.Close()

	imgData, err := io.ReadAll(file)
	if err != nil {
		log.Printf("reading file: %v", err)
		writeErrorCard(w, "Could not read the uploaded image. Please try again.")
		return
	}
	if len(imgData) == 0 {
		writeErrorCard(w, "Uploaded image was empty.")
		return
	}

	mimeType := http.DetectContentType(imgData)
	if !isSupportedImageMIME(mimeType) {
		writeErrorCard(w, "Unsupported file type. Please upload a JPG, PNG, or WebP image.")
		return
	}

	ext := fileExtension(header.Filename, mimeType)
	filename := fmt.Sprintf("%d-%s%s", time.Now().Unix(), uuid.New().String()[:8], ext)
	if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
		log.Printf("creating upload dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(h.uploadDir, filename), imgData, 0644); err != nil {
		log.Printf("writing upload copy: %v", err)
	}

	scanMode := normalizeScanMode(r.FormValue("scan_mode"))

	if scanMode == "bulk" {
		h.handleBulkScan(w, r, imgData, mimeType, filename)
		return
	}

	// Single plant scan
	var previousDiag *string
	plantIDStr := strings.TrimSpace(r.FormValue("plant_id"))
	name := strings.TrimSpace(r.FormValue("name"))
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
		writeErrorCard(w, "Analysis failed. Please try again in a moment.")
		return
	}
	normalizeAnalysisResult(result)

	plantID, err := h.savePlantResult(r, plantIDStr, name, result, filename, imgData, mimeType)
	if err != nil {
		log.Printf("saving result: %v", err)
		writeErrorCard(w, "We analyzed the image but could not save the result. Please try again.")
		return
	}

	h.renderResultCard(w, result, plantID)
}

func (h *Handler) handleBulkScan(w http.ResponseWriter, r *http.Request, imgData []byte, mimeType, filename string) {
	bulk, err := h.gemini.AnalyzeBulk(r.Context(), imgData, mimeType)
	if err != nil {
		log.Printf("Gemini bulk analysis: %v", err)
		writeErrorCard(w, "Bulk analysis failed. Please retry with a clearer photo.")
		return
	}

	if len(bulk.Plants) == 0 {
		writeErrorCard(w, "No plants were detected. Try a clearer photo with each plant visible.")
		return
	}

	totalDetected := bulk.PlantCount
	if totalDetected <= 0 || totalDetected != len(bulk.Plants) {
		totalDetected = len(bulk.Plants)
	}
	fmt.Fprintf(w, `<div class="bulk-results"><div class="bulk-header"><span class="bulk-count">%d</span> plants detected</div>`, totalDetected)

	savedCount := 0
	for i, result := range bulk.Plants {
		normalizeAnalysisResult(&result)
		plantID, err := h.savePlantResult(r, "", "", &result, fmt.Sprintf("%d-%s", i, filename), imgData, mimeType)
		if err != nil {
			log.Printf("saving bulk plant %d: %v", i, err)
			continue
		}
		savedCount++
		h.renderResultCard(w, &result, plantID)
	}

	if savedCount == 0 {
		fmt.Fprintf(w, `<div class="error-card">Detected plants, but none of the results could be saved. Please retry.</div>`)
	} else if savedCount < totalDetected {
		fmt.Fprintf(w, `<div class="error-card">Saved %d of %d detected plants. Retry for the missing entries.</div>`, savedCount, totalDetected)
	}

	fmt.Fprintf(w, `</div>`)
}

func (h *Handler) savePlantResult(r *http.Request, plantIDStr string, explicitName string, result *gemini.AnalysisResult, filename string, imgData []byte, mimeType string) (int, error) {
	if result == nil {
		return 0, fmt.Errorf("analysis result is nil")
	}
	normalizeAnalysisResult(result)

	var plantID int
	if plantIDStr != "" {
		parsedID, err := strconv.Atoi(plantIDStr)
		if err != nil || parsedID <= 0 {
			return 0, fmt.Errorf("invalid plant id %q", plantIDStr)
		}
		if _, err := h.plants.GetByID(r.Context(), parsedID); err != nil {
			return 0, fmt.Errorf("plant %d not found: %w", parsedID, err)
		}
		plantID = parsedID
	} else {
		name := strings.TrimSpace(explicitName)
		if name == "" {
			name = strings.TrimSpace(result.CommonName)
		}
		if name == "" {
			name = strings.TrimSpace(result.Species)
		}
		if name == "" {
			name = "Unknown Plant"
		}

		species := strings.TrimSpace(result.Species)
		commonName := strings.TrimSpace(result.CommonName)
		if commonName == "" {
			commonName = name
		}

		plant, err := h.plants.Create(r.Context(), name, species, commonName)
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

	if _, err := h.assess.Create(r.Context(), plantID, filename, imgData, mimeType, assess); err != nil {
		return 0, fmt.Errorf("creating assessment: %w", err)
	}

	return plantID, nil
}

func (h *Handler) renderResultCard(w http.ResponseWriter, result *gemini.AnalysisResult, plantID int) {
	normalizeAnalysisResult(result)

	commonName := strings.TrimSpace(result.CommonName)
	if commonName == "" {
		commonName = "Unknown Plant"
	}
	species := strings.TrimSpace(result.Species)
	if species == "" {
		species = "Species unavailable"
	}

	urgentHTML := ""
	if result.Urgent != "" {
		urgentHTML = fmt.Sprintf(`<div class="result-urgent"><div class="result-section-title">Urgent</div><p>%s</p></div>`, html.EscapeString(result.Urgent))
	}

	seasonalHTML := ""
	if result.SeasonalAdvice != "" {
		seasonalHTML = fmt.Sprintf(`<div class="result-section"><div class="result-section-title">Seasonal</div><p>%s</p></div>`, html.EscapeString(result.SeasonalAdvice))
	}

	confidenceLabel := normalizeConfidence(result.Confidence)
	confidenceText := confidenceBadgeLabel(confidenceLabel)

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
		html.EscapeString(commonName),
		html.EscapeString(species),
		html.EscapeString(confidenceLabel),
		html.EscapeString(confidenceText),
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

func normalizeScanMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "bulk") {
		return "bulk"
	}
	return "single"
}

func normalizeAnalysisResult(result *gemini.AnalysisResult) {
	if result == nil {
		return
	}

	result.HealthScore = clampScore(result.HealthScore)
	result.SubScores.Foliage = clampScore(result.SubScores.Foliage)
	result.SubScores.Hydration = clampScore(result.SubScores.Hydration)
	result.SubScores.PestRisk = clampScore(result.SubScores.PestRisk)
	result.SubScores.Vitality = clampScore(result.SubScores.Vitality)
	result.Confidence = normalizeConfidence(result.Confidence)
	result.CommonName = strings.TrimSpace(result.CommonName)
	result.Species = strings.TrimSpace(result.Species)
	result.Diagnosis = strings.TrimSpace(result.Diagnosis)
	result.CareTips = strings.TrimSpace(result.CareTips)
	result.Urgent = strings.TrimSpace(result.Urgent)
	result.SeasonalAdvice = strings.TrimSpace(result.SeasonalAdvice)
}

func normalizeConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high":
		return "high"
	case "low":
		return "low"
	default:
		return "medium"
	}
}

func confidenceBadgeLabel(confidence string) string {
	switch confidence {
	case "high":
		return "High"
	case "low":
		return "Low"
	default:
		return "Medium"
	}
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 10 {
		return 10
	}
	return score
}

func fileExtension(originalName, mimeType string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(originalName)))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return ext
	}

	switch mimeType {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

func isSupportedImageMIME(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

func writeErrorCard(w http.ResponseWriter, msg string) {
	fmt.Fprintf(w, `<div class="error-card">%s</div>`, html.EscapeString(msg))
}
