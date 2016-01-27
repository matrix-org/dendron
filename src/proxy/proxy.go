package proxy

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

// An HTTPError is the information needed to make an error response for a
// Matrix client along with the actual error that caused the failure for
// logging.
type HTTPError struct {
	// Err is the root cause of the error for logging
	Err error
	// StatusCode is the HTTP status code to report to the client
	StatusCode int
	// ErrCode is an escaped JSON string to return in the "errcode" part of the
	// JSON response.
	ErrCode string
	// Message is an escaped JSON string to return in the "message" part of the
	// JSON response.
	Message string
}

// LogAndReplyError logs the httpError and writes a JSON formatted error to w.
func LogAndReplyError(w http.ResponseWriter, httpError *HTTPError) {
	log.Printf("%s: %v", httpError.Message, httpError.Err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpError.StatusCode)
	fmt.Fprintf(w, `{"errcode":"%s","error":"%s"}`, httpError.ErrCode, httpError.Message)
}

// A SynapseProxy handles HTTP requests by proxying them to a Synapse server.
type SynapseProxy struct {
	// The URL where proxied requests are sent to.
	URL url.URL
	// The Client used to send proxied requests.
	Client http.Client
}

// ProxyHTTP sends an HTTP request built from the method, url, body, content
// length and headers to a Synapse and writes the response from Synapse to w.
func (p *SynapseProxy) ProxyHTTP(w http.ResponseWriter, method string, url *url.URL, body io.Reader, length int64, headers http.Header) {
	proxyURL := *url
	proxyURL.Scheme = p.URL.Scheme
	proxyURL.Host = p.URL.Host

	req, err := http.NewRequest(method, proxyURL.String(), body)
	if err != nil {
		LogAndReplyError(w, &HTTPError{err, 500, "M_UNKNOWN", "Error proxying request"})
		return
	}

	req.ContentLength = length
	for key, value := range headers {
		req.Header[key] = value
	}

	resp, err := p.Client.Do(req)
	if err != nil {
		LogAndReplyError(w, &HTTPError{err, 500, "M_UNKNOWN", "Error proxying request"})
		return
	}

	defer resp.Body.Close()

	for key, value := range resp.Header {
		w.Header()[key] = value
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *SynapseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	p.ProxyHTTP(w, req.Method, req.URL, req.Body, req.ContentLength, req.Header)
}
