package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
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

func HandleSignal(channel chan os.Signal, synapse *os.Process) {
	select {
	case sig := <-channel:
		log.Print("Got signal: ", sig)
		synapse.Signal(os.Interrupt)
		os.Exit(1)
	}
}

func WaitForSynapse(sp *SynapseProxy) error {
	period := 10 * time.Millisecond
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := sp.Client.Get(sp.URL.String()); err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(period)
		period *= 2
	}
	return fmt.Errorf("Failed to start synapse")
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
	mux.Handle("/", &synapseProxy)
	mux.HandleFunc("/_dendron/test", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintln(w, "test")
	})

	s := &http.Server{
		Addr:           *listenAddr,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
		TLSConfig:      &tls.Config{},
	}

	cert, err := tls.LoadX509KeyPair(*listenCertFile, *listenKeyFile)

	(*s.TLSConfig).Certificates = append((*s.TLSConfig).Certificates, cert)

	synapse := exec.Command(*synapsePython, "-m", "synapse.app.homeserver", "-c", *synapseConfig)
	synapse.Stderr = os.Stderr
	log.Print("Dendron: Starting synapse...")

	synapse.Start()

	channel := make(chan os.Signal, 1)
	signal.Notify(channel, os.Interrupt)
	go HandleSignal(channel, synapse.Process)

	if err := WaitForSynapse(&synapseProxy); err != nil {
		panic(err)
	}

	log.Print("Dendron: Synapse started")

	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		panic(err)
	}

	tlsListener := tls.NewListener(listener, s.TLSConfig)

	go s.Serve(tlsListener)

	if err := synapse.Wait(); err != nil {
		panic(err)
	}
}
