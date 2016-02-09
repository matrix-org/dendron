package proxy

import (
	"fmt"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
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
	log.WithFields(log.Fields{
		"error":      httpError.Err,
		"errMessage": httpError.Message,
		"statusCode": httpError.StatusCode,
		"errCode":    httpError.ErrCode,
	}).Print("Responding with error")
	SetHeaders(w)
	w.WriteHeader(httpError.StatusCode)
	fmt.Fprintf(w, `{"errcode":"%s","error":"%s"}`, httpError.ErrCode, httpError.Message)
}

// MeasureByPath records how long the requests take to process in a histogram labeled by path.
func MeasureByPath(metrics *prometheus.HistogramVec, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		fn(w, req)
		record(metrics, req, start)
	}
}

func record(metrics *prometheus.HistogramVec, req *http.Request, start time.Time) {
	sanitizedPath := endpointFor(req.URL.Path)
	if sanitizedPath == unknownPath {
		log.WithField("path", req.URL.Path).Warn("Proxying unknown path")
	}
	metric, err := metrics.GetMetricWithLabelValues(sanitizedPath, req.Method)
	if err != nil {
		log.WithFields(log.Fields{
			"path":   req.URL.Path,
			"method": req.Method,
		}).Print("Error getting proxy metric")
	} else {
		metric.Observe(float64(time.Now().Sub(start).Nanoseconds() / 1000))
	}
}
