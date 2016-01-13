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
	synapseUrlStr = flag.String("synapse-url", "http://localhost:18448", "Where synapse will be running")
)

var (
	listenAddr     = flag.String("addr", ":8448", "Address to bind to")
	listenCertFile = flag.String("cert-file", "homeserver.crt", "TLS Certificate")
	listenKeyFile  = flag.String("key-file", "homeserver.key", "TLS Private Key")
)

var synapseUrl url.URL

var synapseClient http.Client

func SynapseProxy(w http.ResponseWriter, req *http.Request) {
	url := *req.URL
	url.Scheme = synapseUrl.Scheme
	url.Host = synapseUrl.Host
	var synapseReq *http.Request
	if sreq, err := http.NewRequest(req.Method, url.String(), req.Body); err != nil {
		w.WriteHeader(500)
		fmt.Fprintln(w, "Error:", err)
	} else {
		synapseReq = sreq
	}
	synapseReq.ContentLength = req.ContentLength
	synapseReq.Header["Content-Type"] = req.Header["Content-Type"]
	synapseReq.Header["Content-Disposition"] = req.Header["Content-Dispostion"]
	synapseReq.Header["Authorization"] = req.Header["Authorization"]
	if resp, err := synapseClient.Do(synapseReq); err != nil {
		w.WriteHeader(500)
		fmt.Fprintln(w, "Error:", err)
	} else {
		defer resp.Body.Close()

		w.Header()["Content-Type"] = resp.Header["Content-Type"]
		w.Header()["Content-Disposition"] = resp.Header["Content-Disposition"]
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

func main() {
	flag.Parse()

	if u, err := url.Parse(*synapseUrlStr); err != nil {
		panic(err)
	} else {
		synapseUrl = *u
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", SynapseProxy)
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
