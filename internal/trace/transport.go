package trace

import (
	"net/http"
	"time"
)

// Transport logs each request's outcome and time taken, then hands it to
// base (http.DefaultTransport when nil).
func Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return transport{base}
}

type transport struct{ base http.RoundTripper }

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !On() {
		return t.base.RoundTrip(req)
	}
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	took := time.Since(start).Round(time.Millisecond)
	if err != nil {
		Printf("%s %s ✗ %v (%s)", req.Method, req.URL, err, took)
		return resp, err
	}
	Printf("%s %s → %d (%s)", req.Method, req.URL, resp.StatusCode, took)
	return resp, nil
}
