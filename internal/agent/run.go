package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type service struct {
	collector *Collector
	publisher *Publisher
	root      context.Context
	timeout   time.Duration
	mu        sync.Mutex
	reporting atomic.Bool
	logger    *log.Logger
}

func Main(args []string, version, commit string, stdout, stderr io.Writer) int {
	if len(args) == 2 && (args[1] == "--version" || args[1] == "version") {
		fmt.Fprintf(stdout, "usage-telemetry-agent %s (%s)\n", version, commit)
		return 0
	}
	if len(args) < 2 || args[1] != "client" {
		fmt.Fprintln(stderr, "usage: usage-telemetry-agent client [options]")
		fmt.Fprintln(stderr, "       usage-telemetry-agent --version")
		return 2
	}
	cfg, err := parseConfig(args[2:], os.LookupEnv, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "configuration error: %v\n", err)
		return 2
	}
	api, err := NewAPIClient(cfg.APIURL, cfg.AccessKey, cfg.SecretKey, cfg.RequestTimeout)
	if err != nil {
		fmt.Fprintf(stderr, "API configuration error: %v\n", err)
		return 2
	}
	collector := NewCollector(api, cfg.IncludeInstallationID)
	if cfg.Once {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.RequestTimeout*12)
		defer cancel()
		record, collectErr := collector.Collect(ctx)
		if collectErr != nil {
			fmt.Fprintf(stderr, "collection error: %v\n", collectErr)
			return 1
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(record); err != nil {
			fmt.Fprintf(stderr, "output error: %v\n", err)
			return 1
		}
		return 0
	}

	var publisher *Publisher
	if cfg.TargetURL != "" {
		publisher, err = NewPublisher(cfg.TargetURL, cfg.RequestTimeout, version)
		if err != nil {
			fmt.Fprintf(stderr, "publishing configuration error: %v\n", err)
			return 2
		}
	}
	rootContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := log.New(stderr, "usage-telemetry-agent: ", log.LstdFlags|log.LUTC)
	svc := &service{collector: collector, publisher: publisher, root: rootContext, timeout: cfg.RequestTimeout, logger: logger}

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           svc.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      cfg.RequestTimeout * 12,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	if publisher == nil {
		logger.Printf("local aggregate endpoint enabled on %s; external publishing is disabled", cfg.Listen)
	} else {
		logger.Printf("explicit external publishing is enabled; interval=%s", cfg.Interval)
		go svc.report(rootContext)
		go func() {
			ticker := time.NewTicker(cfg.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-rootContext.Done():
					return
				case <-ticker.C:
					svc.report(rootContext)
				}
			}
		}()
	}

	errChannel := make(chan error, 1)
	go func() {
		errChannel <- server.ListenAndServe()
	}()
	select {
	case <-rootContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			logger.Printf("shutdown failed: %v", err)
			return 1
		}
		return 0
	case err := <-errChannel:
		if err == http.ErrServerClosed {
			return 0
		}
		logger.Printf("HTTP server failed: %v", err)
		return 1
	}
}

func (s *service) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "ok\n")
	})
	mux.HandleFunc("GET /v1-telemetry", s.show)
	mux.HandleFunc("POST /v1-telemetry/reload", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1-telemetry/report", func(writer http.ResponseWriter, request *http.Request) {
		if s.publisher == nil {
			writeJSON(writer, http.StatusOK, map[string]string{"status": "publishing-disabled"})
			return
		}
		go s.report(s.root)
		writeJSON(writer, http.StatusAccepted, map[string]string{"status": "accepted"})
	})
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Security-Policy", "default-src 'none'")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		if request.Method == http.MethodPost && request.Header.Get("Origin") != "" {
			writeJSON(writer, http.StatusForbidden, map[string]string{"type": "error", "message": "browser-origin requests are not accepted"})
			return
		}
		mux.ServeHTTP(writer, request)
	})
}

func (s *service) show(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), s.timeout*12)
	defer cancel()
	s.mu.Lock()
	record, err := s.collector.Collect(ctx)
	s.mu.Unlock()
	if err != nil {
		writeJSON(writer, http.StatusBadGateway, map[string]string{"type": "error", "message": "compatible API queries failed"})
		return
	}
	writeJSON(writer, http.StatusOK, record)
}

func (s *service) report(parent context.Context) {
	if s.publisher == nil {
		return
	}
	if !s.reporting.CompareAndSwap(false, true) {
		return
	}
	defer s.reporting.Store(false)
	ctx, cancel := context.WithTimeout(parent, s.timeout*12)
	defer cancel()
	s.mu.Lock()
	record, err := s.collector.Collect(ctx)
	s.mu.Unlock()
	if err == nil {
		err = s.publisher.Report(ctx, record)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Printf("aggregate report failed: %v", err)
	}
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
