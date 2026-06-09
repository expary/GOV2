package httpapi

import (
	"net/http"
	"sort"
	"strings"
	"sync"
)

const unknownRouteLabel = "unknown"

type httpMetrics struct {
	mu       sync.Mutex
	routes   []metricsRoute
	requests map[requestMetricKey]uint64
}

type metricsRoute struct {
	method string
	path   string
	parts  []string
}

type requestMetricKey struct {
	Method string
	Route  string
	Status int
}

func newHTTPMetrics(routes []Route) *httpMetrics {
	out := &httpMetrics{
		routes:   make([]metricsRoute, 0, len(routes)),
		requests: map[requestMetricKey]uint64{},
	}
	for _, route := range routes {
		if route.Method == "" || route.Path == "" {
			continue
		}
		out.routes = append(out.routes, metricsRoute{
			method: strings.ToUpper(route.Method),
			path:   route.Path,
			parts:  pathParts(route.Path),
		})
	}
	sort.Slice(out.routes, func(i, j int) bool {
		if len(out.routes[i].parts) != len(out.routes[j].parts) {
			return len(out.routes[i].parts) > len(out.routes[j].parts)
		}
		return out.routes[i].path < out.routes[j].path
	})
	return out
}

func (m *httpMetrics) record(method, path string, status int) {
	if m == nil {
		return
	}
	if status == 0 {
		status = http.StatusOK
	}
	key := requestMetricKey{
		Method: strings.ToUpper(method),
		Route:  m.routeFor(method, path),
		Status: status,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[key]++
}

func (m *httpMetrics) snapshot() []requestMetric {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	items := make([]requestMetric, 0, len(m.requests))
	for key, count := range m.requests {
		items = append(items, requestMetric{Key: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Key.Method != items[j].Key.Method {
			return items[i].Key.Method < items[j].Key.Method
		}
		if items[i].Key.Route != items[j].Key.Route {
			return items[i].Key.Route < items[j].Key.Route
		}
		return items[i].Key.Status < items[j].Key.Status
	})
	return items
}

func (m *httpMetrics) routeFor(method, path string) string {
	method = strings.ToUpper(method)
	parts := pathParts(path)
	for _, route := range m.routes {
		if route.method != method || len(route.parts) != len(parts) {
			continue
		}
		if route.matches(parts) {
			return route.path
		}
	}
	if strings.HasPrefix(path, "/api/") {
		return unknownRouteLabel
	}
	return "/"
}

func (r metricsRoute) matches(parts []string) bool {
	for i, part := range r.parts {
		if routePartIsParameter(part) {
			if parts[i] == "" {
				return false
			}
			continue
		}
		if part != parts[i] {
			return false
		}
	}
	return true
}

type requestMetric struct {
	Key   requestMetricKey
	Count uint64
}

func pathParts(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func routePartIsParameter(part string) bool {
	return strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") && len(part) > 2
}

func prometheusLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}
