package handler

import (
	"encoding/json"
	"net/http"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto/response"
)

// Handler serves the liveness probe.
type Handler struct{}

func New() Handler {
	return Handler{}
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response.Health{Status: "ok"})
}
