// Package dto holds the data shapes passed between the project's own layers
// (domain results, repository records, job payloads). HTTP shapes live in
// the request and response subpackages; interfaces live in port/contract.
package dto

// ExampleStatus is what the example domain reports from Ping. Transports
// map it to their own shapes (see port/dto/response).
type ExampleStatus struct {
	// Examples is the number of stored examples; it stays 0 while the
	// service has no storage.
	Examples int
}

// ExampleJob is the payload of one example job: the message body the
// worker consumes and the input the example-job command builds from flags.
type ExampleJob struct {
	// Name identifies the job; the domain rejects an empty name.
	Name string `json:"name"`
}
