package handler

import (
	"log"
	"net/http"
	"strconv"
)

func (h *Handler) PlantDetail(w http.ResponseWriter, r *http.Request) {
	activeProfile, profiles, ok := h.resolveActiveProfile(w, r)
	if !ok {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	plant, err := h.plants.GetByIDForProfile(r.Context(), id, activeProfile.ID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	assessments, err := h.assess.ListByPlant(r.Context(), id)
	if err != nil {
		log.Printf("listing assessments: %v", err)
	}

	h.renderPage(w, "plant.html", map[string]any{
		"Plant":          plant,
		"Assessments":    assessments,
		"CurrentProfile": activeProfile,
		"Profiles":       profiles,
		"CurrentPath":    r.URL.RequestURI(),
	})
}
