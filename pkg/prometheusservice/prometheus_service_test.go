package prometheusservice

import (
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

const upstreamBody = `{"status":"success","data":{"resultType":"vector","result":[]}}`

type recordedRequest struct {
	method string
	path   string
	query  string
	header http.Header
}

func newUpstream(t *testing.T) (*httptest.Server, func() []recordedRequest) {
	var mu sync.Mutex
	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, recordedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			header: r.Header.Clone(),
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			defer gz.Close()
			gz.Write([]byte(upstreamBody))
			return
		}
		w.Write([]byte(upstreamBody))
	}))
	t.Cleanup(server.Close)
	return server, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), requests...)
	}
}

func newRouter(t *testing.T, upstreamURL string) *mux.Router {
	router := mux.NewRouter()
	require.NoError(t, NewPrometheusService(upstreamURL, router))
	return router
}

func TestNewPrometheusServiceForwardsQueries(t *testing.T) {
	upstream, requests := newUpstream(t)
	router := newRouter(t, upstream.URL)

	for _, target := range []string{
		"/api/v1/prometheus/api/v1/query?query=up",
		"/api/v1/prometheus/api/v1/query_range?query=up&start=1&end=2&step=1",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Cookie", "session=secret")
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, target)
		require.Empty(t, rec.Header().Get("Content-Encoding"), target)
		require.JSONEq(t, upstreamBody, rec.Body.String(), target)
	}

	got := requests()
	require.Len(t, got, 2)
	require.Equal(t, "/api/v1/query", got[0].path)
	require.Equal(t, "query=up", got[0].query)
	require.Equal(t, "/api/v1/query_range", got[1].path)
	require.Equal(t, "query=up&start=1&end=2&step=1", got[1].query)
	for _, r := range got {
		require.Equal(t, http.MethodGet, r.method)
		require.Empty(t, r.header.Get("Authorization"))
		require.Empty(t, r.header.Get("Cookie"))
	}
}

func TestNewPrometheusServiceBlocksNonQueryRequests(t *testing.T) {
	upstream, requests := newUpstream(t)
	router := newRouter(t, upstream.URL)

	for _, tc := range []struct {
		method string
		target string
	}{
		{http.MethodPost, "/api/v1/prometheus/-/quit"},
		{http.MethodPut, "/api/v1/prometheus/-/reload"},
		{http.MethodGet, "/api/v1/prometheus/-/quit"},
		{http.MethodPost, "/api/v1/prometheus/api/v1/otlp/v1/metrics"},
		{http.MethodPost, "/api/v1/prometheus/api/v1/write"},
		{http.MethodPost, "/api/v1/prometheus/api/v1/admin/tsdb/delete_series?match[]=up"},
		{http.MethodGet, "/api/v1/prometheus/api/v1/status/flags"},
		{http.MethodGet, "/api/v1/prometheus/api/v1/targets"},
		{http.MethodPost, "/api/v1/prometheus/api/v1/query"},
		{http.MethodDelete, "/api/v1/prometheus/api/v1/query_range"},
		{http.MethodGet, "/api/v1/prometheus/api/v1/query/../../../-/quit"},
		{http.MethodGet, "/api/v1/prometheus/api/v1/query/extra"},
	} {
		req := httptest.NewRequest(tc.method, tc.target, strings.NewReader("x"))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.NotEqual(t, http.StatusOK, rec.Code, "%s %s", tc.method, tc.target)
	}

	require.Empty(t, requests())
}

func TestNewPrometheusServiceRejectsInvalidURL(t *testing.T) {
	for _, u := range []string{
		"://bad",
		"ftp://prometheus:9090",
		"http://",
	} {
		require.Error(t, NewPrometheusService(u, mux.NewRouter()), u)
	}
}
