package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"plenilune/internal/demohttp"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Printf("demo server: %v", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:8080", "loopback listen address")
	webDir := flag.String("web", "web/dist", "built frontend directory")
	flag.Parse()
	if !loopbackAddress(*addr) {
		return errors.New("demo server only listens on a loopback address")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := newServer(*addr, demohttp.NewHandlerForAddr(*webDir, *addr))
	errCh := make(chan error, 1)
	go func() {
		log.Printf("Plenilune demo listening on http://%s", *addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// newServer builds the demo HTTP server with bounded header, read and write
// phases so a slow or stalled client cannot hold a connection forever.
// ReadTimeout (60s) covers headers plus the largest accepted body (a replay
// request can reach ~26 MiB, still seconds over loopback) and bounds
// slow-body drips. WriteTimeout (180s) must outlast the longest legitimate
// handler: one analysis may run up to the 120s maximum analysis deadline
// before its response is written. MaxHeaderBytes is set explicitly to the
// net/http default so the bound is visible rather than implied.
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

func loopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}
