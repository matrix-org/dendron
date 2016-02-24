package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/http/pprof"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	stdlog "log"

	log "github.com/Sirupsen/logrus"

	"github.com/matrix-org/dendron/login"
	"github.com/matrix-org/dendron/proxy"
	"github.com/matrix-org/dendron/versions"

	"github.com/matrix-org/dugong"
	"github.com/prometheus/client_golang/prometheus"

	_ "github.com/lib/pq" /* Database driver for postgres */
)

var (
	startSynapse   = flag.Bool("start-synapse", true, "Start a synapse process, otherwise connect to an existing synapse")
	synapseConfig  = flag.String("synapse-config", "homeserver.yaml", "Path to synapse's config")
	synapsePython  = flag.String("synapse-python", "python", "A python interpreter to use for synapse. This should be the python binary installed inside synapse's virtualenv. The interpreter will be looked up on the $PATH")
	synapseURLStr  = flag.String("synapse-url", "http://localhost:18448", "The HTTP URL that synapse is configured to listen on.")
	synapseDB      = flag.String("synapse-postgres", "", "Database config for the postgresql as per https://godoc.org/github.com/lib/pq#hdr-Connection_String_Parameters. This must point to the same database that synapse is configured to use")
	serverName     = flag.String("server-name", "", "Matrix server name. This must match the server_name configured for synapse.")
	macaroonSecret = flag.String("macaroon-secret", "", "Secret key for macaroons. This must match the macaroon_secret_key configured for synapse.")
	listenAddr     = flag.String("addr", ":8448", "Address to listen for matrix requests on")
	listenTLS      = flag.Bool("tls", true, "Listen for HTTPS requests, otherwise listen for HTTP requests")
	listenCertFile = flag.String("cert-file", "", "TLS Certificate. This must match the tls_certificate_path configured for synapse.")
	listenKeyFile  = flag.String("key-file", "", "TLS Private Key. The private key for the certificate. This must be set if listening for HTTPS requests")

	logDir = flag.String("log-dir", "var", "Logging output directory, Dendron logs to error.log, warn.log and info.log in that directory")
)

func handleSignal(channel chan os.Signal, synapse *os.Process, synapseLog *log.Entry) {
	select {
	case sig := <-channel:
		log.WithField("signal", sig).Print("Got signal")
		synapseLog.Print("Killing synapse")
		synapse.Signal(os.Interrupt)
		os.Exit(1)
	}
}

func waitForSynapse(synapseURL *url.URL, synapseLog *log.Entry) error {
	synapseLog.Print("Connecting to synapse")
	period := 50 * time.Millisecond
	timeout := 20 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(synapseURL.String()); err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(period)
	}

	return fmt.Errorf("timeout waiting for synapse to start")
}

func setMaxOpenFiles() (uint64, error) {
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return 0, err
	}
	limit.Cur = limit.Max
	return limit.Max, syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit)
}

func main() {
	flag.Parse()

	log.AddHook(dugong.NewFSHook(
		filepath.Join(*logDir, "info.log"),
		filepath.Join(*logDir, "warn.log"),
		filepath.Join(*logDir, "error.log"),
	))

	if noFiles, err := setMaxOpenFiles(); err != nil {
		panic(err)
	} else {
		log.WithField("files", noFiles).Printf("Set maximum number of open files")
	}

	proxyMetrics := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "synapse_proxy_request_durations_microseconds",
			Help: "Histogram of microsecond durations of requests to underlying synapse for proxied requests",

			// Manually curated list of expected request timings.
			// Ranges from <1ms to <2 minutes, and then an auto-generated >2 minutes bucket.
			Buckets: []float64{
				// <1s
				1000, 10000, 25000, 50000, 75000, 100000,
				// <10s
				1000000, 1250000, 1500000, 1750000, 2000000, 3000000, 4000000, 5000000,
				// <60s
				10000000, 20000000, 30000000, 45000000,
				// >= 60s
				60000000, 120000000,
			},
		},
		[]string{"path", "method"},
	)
	prometheus.MustRegister(proxyMetrics)

	synapseURL, err := url.Parse(*synapseURLStr)
	if err != nil {
		panic(err)
	}

	var synapse *exec.Cmd

	var synapseLog = log.WithField("synapse", synapseURL.String())

	if *startSynapse {
		synapse = exec.Command(*synapsePython, "-m", "synapse.app.homeserver", "-c", *synapseConfig)
		synapse.Stderr = os.Stderr
		synapseLog.Print("Starting synapse")

		synapse.Start()

		channel := make(chan os.Signal, 1)
		signal.Notify(channel, os.Interrupt)
		go handleSignal(channel, synapse.Process, synapseLog)

		if err := waitForSynapse(synapseURL, synapseLog); err != nil {
			synapseLog.Panic(err)
		}

		synapseLog.Print("Synapse started")
	} else {
		synapseLog.Print("Using existing synapse")
	}

	db, err := sql.Open("postgres", *synapseDB)
	if err != nil {
		panic(err)
	}

	reverseProxy := proxy.MeasureByPath(proxyMetrics, httputil.NewSingleHostReverseProxy(synapseURL).ServeHTTP)

	loginHandler, err := login.NewHandler(db, reverseProxy, *serverName, *macaroonSecret)
	if err != nil {
		panic(err)
	}

	versionsHandler, err := versions.NewHandler(synapseURL, time.Hour)
	if err != nil {
		panic(err)
	}

	loginFunc := prometheus.InstrumentHandler("login", loginHandler)
	proxyFunc := prometheus.InstrumentHandler("proxy", reverseProxy)
	versionsFunc := prometheus.InstrumentHandler("versions", versionsHandler)

	mux := http.NewServeMux()
	mux.Handle("/", proxyFunc)
	mux.Handle("/_matrix/client/api/v1/login", loginFunc)
	mux.Handle("/_matrix/client/r0/login", loginFunc)
	mux.Handle("/_matrix/client/versions", versionsFunc)
	mux.HandleFunc("/_dendron/test", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintln(w, "test")
	})
	mux.Handle("/_dendron/metrics", prometheus.Handler())

	// The debug pprof handlers have to be hosted under "/debug/pprof" because
	// that string is hardcoded inside them.
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	logWriter := log.StandardLogger().Writer()
	defer logWriter.Close()
	s := &http.Server{
		Addr:           *listenAddr,
		Handler:        mux,
		ReadTimeout:    5 * time.Minute,
		WriteTimeout:   5 * time.Minute,
		MaxHeaderBytes: 1 << 20,
		ErrorLog:       stdlog.New(logWriter, "", 0),
	}

	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		panic(err)
	}

	if *listenTLS {
		cert, err := tls.LoadX509KeyPair(*listenCertFile, *listenKeyFile)
		if err != nil {
			panic(err)
		}

		s.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}

		listener = tls.NewListener(listener, s.TLSConfig)
	}

	go s.Serve(listener)

	if synapse != nil {
		if err := synapse.Wait(); err != nil {
			panic(err)
		}
	} else {
		select {}
	}
}
