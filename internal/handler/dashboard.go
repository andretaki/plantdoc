package handler

import (
	"log"
	"net/http"

	"github.com/andre/plantdoc/internal/model"
)

type PlantCard struct {
	Plant       model.Plant
	LatestPhoto string
	HealthScore int
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	plants, err := h.plants.List(r.Context())
	if err != nil {
		log.Printf("listing plants: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var cards []PlantCard
	for _, p := range plants {
		card := PlantCard{Plant: p}
		if latest, err := h.assess.GetLatestByPlant(r.Context(), p.ID); err == nil {
			card.LatestPhoto = latest.PhotoPath
			card.HealthScore = latest.HealthScore
		}
		cards = append(cards, card)
	}

	h.renderPage(w, "dashboard.html", map[string]any{
		"Plants": cards,
	})
}
