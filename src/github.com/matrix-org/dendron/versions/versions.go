package versions

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"sync/atomic"
	"time"

	"github.com/matrix-org/dendron/proxy"
)

// NewHandler creates an http.Handler which serves up the client-server API versions currently served by the delegated Synapse.
// It caches this response for updateInterval, and will serve stale cache entries if it cannot get a new value from the Synapse.
func NewHandler(p *proxy.SynapseProxy, updateInterval time.Duration) (*handler, error) {
	h := &handler{p: p}
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

type handler struct {
	p *proxy.SynapseProxy

	resp atomic.Value // Always contains a valid []byte
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	proxy.SetHeaders(w)
	w.Write(h.resp.Load().([]byte))
}

func (h *handler) update() error {
	resp, err := h.p.Client.Get(h.p.URL.String() + "/_matrix/client/versions")
	if err != nil {
		log.Printf("Error updating /version: %v", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respBytes, _ := httputil.DumpResponse(resp, true)
		err := fmt.Errorf("error updating /version: status code: %v, response: %v", resp.StatusCode, string(respBytes))
		log.Print(err)
		return err
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	h.resp.Store(bytes)
	return nil
}
