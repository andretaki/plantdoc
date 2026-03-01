package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/andre/plantdoc/pkg/model"
)

const profileCookieName = "plantdoc_profile_id"

func (h *Handler) resolveActiveProfile(w http.ResponseWriter, r *http.Request) (*model.Profile, []model.Profile, bool) {
	profiles, err := h.profiles.List(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return nil, nil, false
	}

	if len(profiles) == 0 {
		p, err := h.profiles.Create(r.Context(), "Shared Garden")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return nil, nil, false
		}
		profiles = []model.Profile{*p}
	}

	active := profiles[0]
	if c, err := r.Cookie(profileCookieName); err == nil {
		if id, convErr := strconv.Atoi(c.Value); convErr == nil && id > 0 {
			for _, p := range profiles {
				if p.ID == id {
					active = p
					break
				}
			}
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     profileCookieName,
		Value:    strconv.Itoa(active.ID),
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})

	return &active, profiles, true
}

func (h *Handler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Profile name is required", http.StatusBadRequest)
		return
	}
	if len(name) > 80 {
		http.Error(w, "Profile name is too long", http.StatusBadRequest)
		return
	}

	profile, err := h.profiles.Create(r.Context(), name)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     profileCookieName,
		Value:    strconv.Itoa(profile.ID),
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})

	http.Redirect(w, r, safeReturnPath(r), http.StatusSeeOther)
}

func (h *Handler) SelectProfile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	profileID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("profile_id")))
	if err != nil || profileID <= 0 {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if _, err := h.profiles.GetByID(r.Context(), profileID); err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     profileCookieName,
		Value:    strconv.Itoa(profileID),
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})

	http.Redirect(w, r, safeReturnPath(r), http.StatusSeeOther)
}

func safeReturnPath(r *http.Request) string {
	path := strings.TrimSpace(r.FormValue("return_to"))
	if path == "" {
		path = strings.TrimSpace(r.Referer())
	}
	if path == "" {
		return "/"
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Sprintf("/%s", path)
	}
	return path
}
