package versions

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/matrix-org/dendron/proxy"
)

// NewHandler creates an http.Handler which serves up the client-server API versions currently served by the delegated Synapse.
// It caches this response for updateInterval, and will serve stale cache entries if it cannot get a new value from the Synapse.
func NewHandler(synapseURL *url.URL, updateInterval time.Duration) (*Handler, error) {
	h := &Handler{synapseURL: synapseURL}
	if err := h.update(); err != nil {
		return nil, fmt.Errorf("error getting initial version: %v", err)
	}

	go func() {
		for {
			select {
			case <-time.After(updateInterval):
				h.update()
			}
		}
	}()

	return h, nil
}

// Handler handles requests for /_matrix/client/versions by caching the
// response from synapse
type Handler struct {
	synapseURL *url.URL

	resp atomic.Value // Always contains a valid []byte
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	proxy.SetHeaders(w)
	w.Write(h.resp.Load().([]byte))
}

func (h *Handler) update() error {
	url := h.synapseURL.String() + "/_matrix/client/versions"
	resp, err := http.Get(url)
	if err != nil {
		log.WithFields(log.Fields{
			"versionUrl": url,
			"error":      err,
		}).Error("Error updating /version")
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respBytes, _ := httputil.DumpResponse(resp, true)
		log.WithFields(log.Fields{
			"versionUrl":   url,
			"statusCode":   resp.StatusCode,
			"responseBody": string(respBytes),
		}).Error("Non-200 response updating /version")
		return fmt.Errorf("error updating /version: status code: %v, response: %v", resp.StatusCode, string(respBytes))
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	h.resp.Store(bytes)
	return nil
}
