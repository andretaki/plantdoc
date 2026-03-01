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
		fmt.Fprintf(w, `<div class="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700">
			Analysis failed: %s. Your photo was saved - try again later.
		</div>`, html.EscapeString(err.Error()))
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
	</div>`,
		html.EscapeString(result.CommonName),
		html.EscapeString(result.Species),
		result.HealthScore,
		html.EscapeString(result.Diagnosis),
		html.EscapeString(result.CareTips),
		plantID)
}
