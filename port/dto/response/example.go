package response

// Example is the body of GET /example.
type Example struct {
	Message  string `json:"message"`
	Examples int    `json:"examples"`
}
