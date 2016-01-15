package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"
)

var (
	synapseConfig = flag.String("synapse-config", "homeserver.yaml", "Path to a synapse config")
	synapsePython = flag.String("synapse-python", "python", "Path to a python interpreter")
	synapseURL    = flag.String("synapse-url", "http://localhost:18448", "Where synapse will be running")
)

var (
	listenAddr     = flag.String("addr", ":8448", "Address to bind to")
	listenCertFile = flag.String("cert-file", "homeserver.crt", "TLS Certificate")
	listenKeyFile  = flag.String("key-file", "homeserver.key", "TLS Private Key")
)

type SynapseProxy struct {
	URL    url.URL
	Client http.Client
}

func (p *SynapseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	url := *req.URL
	url.Scheme = p.URL.Scheme
	url.Host = p.URL.Host
	synapseReq, err := http.NewRequest(req.Method, url.String(), req.Body)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintln(w, "Error:", err)
		return
	}
	synapseReq.ContentLength = req.ContentLength
	for key, value := range req.Header {
		synapseReq.Header[key] = value
	}
	resp, err := p.Client.Do(synapseReq)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintln(w, "Error:", err)
		return
	}
	defer resp.Body.Close()

	for key, value := range resp.Header {
		w.Header()[key] = value
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func main() {
	flag.Parse()

	var synapseProxy SynapseProxy

	if u, err := url.Parse(*synapseURL); err != nil {
		panic(err)
	} else {
		synapseProxy.URL = *u
	}

	mux := http.NewServeMux()
	mux.Handle("/", synapseProxy)
	mux.HandleFunc("/_dendron/test", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintln(w, "test")
	})

	s := &http.Server{
		Addr:           *listenAddr,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go s.ListenAndServeTLS(*listenCertFile, *listenKeyFile)

	synapse := exec.Command(*synapsePython, "-m", "synapse.app.homeserver", "-c", *synapseConfig)
	synapse.Stderr = os.Stderr
	fmt.Fprintln(os.Stderr, "Dendron: Starting synapse...")

	synapse.Start()
	if err := synapse.Wait(); err != nil {
		panic(err)
	}
}
