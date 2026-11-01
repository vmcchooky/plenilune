package demohttp

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// maxConcurrentAnalyses bounds simultaneous heavy API work (analysis,
// comparison, repair, replay, agent session). The plan starts the limit at 2:
// the demo is a single-user local tool, and one in-flight request plus one
// spare slot covers the UI's sequential flow without letting a burst of
// cross-site or scripted requests hold unbounded CPU.
const maxConcurrentAnalyses = 2

// localOnly guards every /api/* route against DNS rebinding and cross-site
// request forgery against the loopback server. Loopback binding alone is not
// a boundary: a malicious page can resolve its own hostname to 127.0.0.1 and
// reach this server with a foreign Host, so the request's Host, Origin and
// Fetch Metadata are checked instead. expectedPort is the server's listen
// port when known ("" when the handler is used without one, e.g. in tests);
// when known, the Host authority must carry that exact port. An Origin on
// any loopback port stays acceptable: the local dev UI may be served from a
// different loopback port than the API (see TestAIOriginAndMissingProvider).
// Requests without an Origin (curl, the example client) are unaffected, and
// non-API paths (the static UI) pass through.
func localOnly(expectedPort string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && !localRequest(r, expectedPort) {
			writeError(w, http.StatusForbidden, apiError{Code: "LOCAL_ORIGIN_REQUIRED", Message: "API requests must come from the local demo on this machine"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func localRequest(r *http.Request, expectedPort string) bool {
	host, port := splitAuthority(r.Host)
	if !loopbackHost(host) {
		return false
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	if expectedPort != "" {
		effective := port
		if effective == "" {
			effective = "80" // plain-HTTP default when Host omits the port
		}
		if effective != expectedPort {
			return false
		}
	}
	if origin := r.Header.Get("Origin"); origin != "" && !loopbackOrigin(origin) {
		return false
	}
	// Fetch Metadata: only an explicit cross-site marker is rejected; the
	// header is absent for non-browser clients and same-site loopback pages
	// (e.g. a dev UI on another port) stay acceptable.
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	return true
}

// splitAuthority splits a Host/authority value into hostname and port; a
// value without a port yields an empty port.
func splitAuthority(authority string) (host, port string) {
	if h, p, err := net.SplitHostPort(authority); err == nil {
		return h, p
	}
	return authority, ""
}

// loopbackOrigin reports whether an Origin header value is an http(s) origin
// on a loopback host without userinfo. The port is not restricted (see
// localOnly).
func loopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && loopbackHost(u.Hostname())
}

// limitConcurrent wraps a heavy handler with a non-blocking semaphore:
// requests beyond the cap are rejected immediately with a typed 429 instead
// of queueing without bound.
func limitConcurrent(sem chan struct{}, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			next(w, r)
		default:
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, apiError{Code: "SERVER_BUSY", Message: "too many concurrent analyses; retry shortly"})
		}
	}
}
