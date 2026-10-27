package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto/response"
)

// Handler serves the example route. It depends on the ExampleService
// contract, so tests can pass a mock (see mock/example).
type Handler struct {
	exampleSvc contract.ExampleService
	logger     *slog.Logger
}

func New(exampleSvc contract.ExampleService, logger *slog.Logger) Handler {
	return Handler{exampleSvc: exampleSvc, logger: logger}
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status, err := h.exampleSvc.Ping(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "example.ping", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toExampleResponse(status))
}

// toExampleResponse maps the domain status to the response DTO.
func toExampleResponse(status dto.ExampleStatus) response.Example {
	return response.Example{Message: "pong", Examples: status.Examples}
}
