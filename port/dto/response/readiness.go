package response

// Readiness is the body of GET /health/ready.
type Readiness struct {
	// Status is "ok" when every check passed and "unavailable" otherwise.
	Status string `json:"status"`
	// Failed names the checks that failed or timed out, sorted.
	Failed []string `json:"failed,omitempty"`
}
