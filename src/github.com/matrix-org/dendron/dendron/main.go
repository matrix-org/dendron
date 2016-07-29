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
	startSynapse           = flag.Bool("start-synapse", true, "Start a synapse process, otherwise connect to an existing synapse")
	synapseConfig          = flag.String("synapse-config", "homeserver.yaml", "Path to synapse's config")
	synapsePython          = flag.String("synapse-python", "python", "A python interpreter to use for synapse. This should be the python binary installed inside synapse's virtualenv. The interpreter will be looked up on the $PATH")
	synapseURLStr          = flag.String("synapse-url", "http://localhost:18448", "The HTTP URL that synapse is configured to listen on.")
	synapseDB              = flag.String("synapse-postgres", "", "Database config for the postgresql as per https://godoc.org/github.com/lib/pq#hdr-Connection_String_Parameters. This must point to the same database that synapse is configured to use")
	serverName             = flag.String("server-name", "", "Matrix server name. This must match the server_name configured for synapse.")
	macaroonSecret         = flag.String("macaroon-secret", "", "Secret key for macaroons. This must match the macaroon_secret_key configured for synapse.")
	listenAddr             = flag.String("addr", ":8448", "Address to listen for matrix requests on")
	listenTLS              = flag.Bool("tls", true, "Listen for HTTPS requests, otherwise listen for HTTP requests")
	listenCertFile         = flag.String("cert-file", "", "TLS Certificate. This must match the tls_certificate_path configured for synapse.")
	listenKeyFile          = flag.String("key-file", "", "TLS Private Key. The private key for the certificate. This must be set if listening for HTTPS requests")
	pusherConfig           = flag.String("pusher-config", "", "Pusher worker config")
	synchrotronConfig      = flag.String("synchrotron-config", "", "Synchrotron worker config")
	synchrotronURLStr      = flag.String("synchrotron-url", "", "The HTTP URL that the synchrotron will listen on")
	federationReaderConfig = flag.String("federation-reader-config", "", "Federation reader worker config")
	federationReaderURLStr = flag.String("federation-reader-url", "", "The HTTP URL that the federation reader will listen on")

	logDir = flag.String("log-dir", "var", "Logging output directory, Dendron logs to error.log, warn.log and info.log in that directory")
)

func startProcess(app string, processURL *url.URL, terminate chan<- string, name string, args ...string) (*log.Entry, func(), error) {
	process := exec.Command(name, args...)
	process.Stderr = os.Stderr
	processLog := log.WithField("app", app)

	if processURL != nil {
		processLog = processLog.WithField("processURL", processURL.String())
	}

	processLog.Print("Starting process")

	if err := process.Start(); err != nil {
		return processLog, nil, err
	}

	if processURL != nil {
		if err := waitForProcess(processURL, processLog); err != nil {
			return processLog, nil, err
		}
	}

	go func() {
		// Wait for process to stop.
		if _, err := process.Process.Wait(); err != nil {
			processLog.WithError(err).Print("Error waiting for process")
		}
		terminate <- fmt.Sprintf("Process %s Stopped", app)
	}()

	cleanup := func() {
		processLog.Print("Stopping process")

		go func() {
			// Give the process ten seconds to shutdown cleanly.
			time.Sleep(10 * time.Second)
			processLog.Print("Process failed to stop within 10 seconds")
			process.Process.Signal(syscall.SIGKILL)
		}()

		if err := process.Process.Signal(syscall.SIGTERM); err != nil {
			processLog.WithError(err).Print("Failed to kill process")
		}
		process.Process.Wait()
	}

	return processLog, cleanup, nil
}

func waitForProcess(processURL *url.URL, processLog *log.Entry) error {
	processLog.Print("Connecting to process")
	period := 50 * time.Millisecond
	timeout := 20 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(processURL.String()); err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(period)
	}

	return fmt.Errorf("timeout waiting for process to accept http connections")
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

	var synchrotronURL *url.URL
	if *synchrotronURLStr != "" {
		synchrotronURL, err = url.Parse(*synchrotronURLStr)
		if err != nil {
			panic(err)
		}
	}

	var federationReaderURL *url.URL
	if *federationReaderURLStr != "" {
		federationReaderURL, err = url.Parse(*federationReaderURLStr)
		if err != nil {
			panic(err)
		}
	}

	var synapseLog = log.WithFields(log.Fields{
		"processURL": synapseURL.String(),
	})

	// Used to terminate dendron.
	terminate := make(chan string, 1)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		s := <-signals
		terminate <- fmt.Sprintf("Got signal %v", s)
	}()

	if *startSynapse {
		processLog, cleanup, err := startProcess(
			"synapse", synapseURL, terminate,
			*synapsePython,
			"-m", "synapse.app.homeserver",
			"-c", *synapseConfig,
		)

		synapseLog = processLog

		if err != nil {
			processLog.Panic(err)
		}

		defer cleanup()

		if *pusherConfig != "" {
			processLog, cleanup, err := startProcess(
				"pusher", nil, terminate,
				*synapsePython,
				"-m", "synapse.app.pusher",
				"-c", *synapseConfig,
				"-c", *pusherConfig,
			)

			if err != nil {
				processLog.Panic(err)
			}

			defer cleanup()
		}

		if *synchrotronConfig != "" {
			processLog, cleanup, err := startProcess(
				"synchrotron", synchrotronURL, terminate,
				*synapsePython,
				"-m", "synapse.app.synchrotron",
				"-c", *synapseConfig,
				"-c", *synchrotronConfig,
			)

			if err != nil {
				processLog.Panic(err)
			}

			defer cleanup()
		}

		if *federationReaderConfig != "" {
			processLog, cleanup, err := startProcess(
				"federationReader", federationReaderURL, terminate,
				*synapsePython,
				"-m", "synapse.app.federation_reader",
				"-c", *synapseConfig,
				"-c", *federationReaderConfig,
			)

			if err != nil {
				processLog.Panic(err)
			}

			defer cleanup()
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

	if synchrotronURL != nil {
		synchrotronReverseProxy := proxy.MeasureByPath(
			proxyMetrics,
			httputil.NewSingleHostReverseProxy(synchrotronURL).ServeHTTP,
		)
		synchrotronFunc := prometheus.InstrumentHandler(
			"synchrotron", synchrotronReverseProxy,
		)
		mux.Handle("/_matrix/client/v2_alpha/sync", synchrotronFunc)
		mux.Handle("/_matrix/client/r0/sync", synchrotronFunc)
	}

	if federationReaderURL != nil {
		federationReaderReverseProxy := proxy.MeasureByPath(
			proxyMetrics,
			httputil.NewSingleHostReverseProxy(federationReaderURL).ServeHTTP,
		)
		federationReaderFunc := prometheus.InstrumentHandler(
			"federationReader", federationReaderReverseProxy,
		)
		mux.Handle("/_matrix/federation/v1/event/", federationReaderFunc)
		mux.Handle("/_matrix/federation/v1/state/", federationReaderFunc)
		mux.Handle("/_matrix/federation/v1/backfill/", federationReaderFunc)
		mux.Handle("/_matrix/federation/v1/get_missing_events/", federationReaderFunc)
		mux.Handle("/_matrix/federation/v1/publicRooms", federationReaderFunc)
	}

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

	reason := <-terminate

	log.WithField("reason", reason).Print("Shutting Down")
}
