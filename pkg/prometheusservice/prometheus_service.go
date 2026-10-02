package prometheusservice

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gorilla/mux"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const routePrefix = "/api/v1/prometheus"

// Only read-only query endpoints are proxied. Prometheus may run with
// --web.enable-lifecycle or the OTLP/remote-write receivers enabled, and
// exposing those through the portal would let any user (or a cross-site
// form POST) shut it down or inject series.
var allowedPaths = []string{
	"/api/v1/query",
	"/api/v1/query_range",
}

// NewPrometheusService registers a reverse-proxy handler for GET requests
// to /api/v1/prometheus/api/v1/{query,query_range} that forwards them to
// the configured Prometheus URL. This lets the frontend query Prometheus
// without direct cluster access or CORS issues.
func NewPrometheusService(prometheusURL string, router *mux.Router) error {
	target, err := url.Parse(prometheusURL)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid prometheus_url %q: %v", prometheusURL, err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return status.Errorf(codes.InvalidArgument, "invalid prometheus_url %q: scheme must be http or https", prometheusURL)
	}
	if target.Host == "" {
		return status.Errorf(codes.InvalidArgument, "invalid prometheus_url %q: missing host", prometheusURL)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Strip browser auth headers — Prometheus doesn't need them and we
		// shouldn't forward credentials to an internal service.
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		// Strip Accept-Encoding so Prometheus returns plain JSON; otherwise
		// the compressed response passes through undecoded to the browser.
		req.Header.Del("Accept-Encoding")
	}
	// Strip the /api/v1/prometheus prefix before forwarding so the upstream
	// Prometheus receives its native /api/v1/... paths.
	handler := http.StripPrefix(routePrefix, proxy)
	for _, path := range allowedPaths {
		router.Handle(routePrefix+path, handler).Methods(http.MethodGet)
	}
	return nil
}
