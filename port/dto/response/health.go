package response

// Health is the body of GET /health.
type Health struct {
	Status string `json:"status"`
}
