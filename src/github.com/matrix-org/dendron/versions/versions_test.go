package versions

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matrix-org/dendron/proxy"
)

type StaticHandler struct {
	firstResponse  []byte
	secondResponse []byte
	requests       int32
}

func (s *StaticHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req := atomic.AddInt32(&s.requests, 1); req == 1 {
		w.Write(s.firstResponse)
	} else if req == 2 {
		w.Write(s.secondResponse)
	} else {
		w.WriteHeader(500)
		w.Write([]byte("ISE! ISE!"))
	}
}

func TestVersions(t *testing.T) {
	mockHandler := &StaticHandler{
		firstResponse:  []byte("foo"),
		secondResponse: []byte("bar"),
	}
	s := httptest.NewServer(mockHandler)
	defer s.Close()

	u, _ := url.Parse(s.URL)
	p := &proxy.SynapseProxy{
		URL:    *u,
		Client: http.Client{},
	}
	h, err := NewHandler(p, 25*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if val := h.resp.Load(); !reflect.DeepEqual(val, mockHandler.firstResponse) {
		t.Fatalf("first response: want %q got %q", string(mockHandler.firstResponse), string(val.([]byte)))
	}

	sleepAndWantSecond := func(prefix string) {
		time.Sleep(40 * time.Millisecond)
		if val := h.resp.Load(); !reflect.DeepEqual(val, mockHandler.secondResponse) {
			t.Fatalf("%s: want %q got %q", prefix, string(mockHandler.secondResponse), string(val.([]byte)))
		}
	}

	sleepAndWantSecond("second response")
	sleepAndWantSecond("after 500")
	s.Close()
	sleepAndWantSecond("after ECONNREFUSED")
}
