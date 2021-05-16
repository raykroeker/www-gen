package www

// Monitor is a serializable configuration for endpoints to monitor and the
// execution result.
type Monitor struct {
	// Endpoints to monitor.
	Endpoints []*Endpoint `json:"endpoints"`
}

// Endpoint to monitor.
type Endpoint struct {
	// Method to use when making a request.
	Method string `json:"method"`
	// URL to issue a request against (monitor).
	URL string `json:"url"`
	// ExpectedStatusCode expected back.
	ExpectedStatusCode int `json:"expected-status-code"`
	// ExpectedBodyHash base64 encoded.
	ExpectedBodyHash string `json:"expected-body-hash"`
}
