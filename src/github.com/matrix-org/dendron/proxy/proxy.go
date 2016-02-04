package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	log "github.com/Sirupsen/logrus"
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

// SetHeaders sets the "Content-Type" to "application/json" and sets CORS
// headers so that arbitrary sites can use the APIs.
func SetHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
}

// LogAndReplyError logs the httpError and writes a JSON formatted error to w.
func LogAndReplyError(w http.ResponseWriter, httpError *HTTPError) {
	log.Printf("%s: %v", httpError.Message, httpError.Err)
	SetHeaders(w)
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

	if resp.ContentLength != -1 {
		w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	for key, value := range resp.Header {
		w.Header()[key] = value
	}
	w.WriteHeader(resp.StatusCode)

	written, err := io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error writing response (%d bytes written out of %d): %v", written, resp.ContentLength, err)
	}
}

func (p *SynapseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	p.ProxyHTTP(w, req.Method, req.URL, req.Body, req.ContentLength, req.Header)
}
