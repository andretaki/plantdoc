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

	h.renderPage(w, "plant.html", map[string]any{
		"Plant":       plant,
		"Assessments": assessments,
	})
}
