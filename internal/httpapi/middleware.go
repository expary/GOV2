package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

const requestIDHeader = "X-Request-ID"
const maxRequestIDLength = 128
const allowedCORSMethods = "GET,POST,PUT,PATCH,DELETE,OPTIONS"
const allowedCORSHeaders = "Authorization,Content-Type,X-Request-ID"

var allowedCORSMethodSet = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodOptions: {},
}

var allowedCORSHeaderSet = map[string]struct{}{
	"authorization": {},
	"content-type":  {},
	"x-request-id":  {},
}

type requestIDKey struct{}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := contextWithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '-', '_', '.', ':':
			continue
		default:
			return false
		}
	}
	return true
}

func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered", "error", recovered, "stack", string(debug.Stack()))
					writeError(w, r, http.StatusInternalServerError, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) StatusCode() int {
	return w.status
}

type statusCodeWriter interface {
	StatusCode() int
}

func responseStatus(w http.ResponseWriter, fallback int) int {
	if sw, ok := w.(statusCodeWriter); ok {
		return sw.StatusCode()
	}
	return fallback
}

func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			status := responseStatus(sw, sw.status)
			logger.Info("http request",
				"request_id", requestIDFromContext(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

func recordHTTPMetrics(metrics *httpMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			if metrics != nil {
				metrics.record(r.Method, r.URL.Path, responseStatus(sw, sw.status))
			}
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func cors(allowedOrigins []string) func(http.Handler) http.Handler {
	allowedOrigins = normalizeOrigins(allowedOrigins)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			allowedOrigin, allowed := allowedCORSOrigin(allowedOrigins, origin)
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				if allowedOrigin != "*" {
					w.Header().Add("Vary", "Origin")
				}
			} else if origin != "" && r.Method == http.MethodOptions {
				writeError(w, r, http.StatusForbidden, "origin not allowed")
				return
			}

			w.Header().Set("Access-Control-Allow-Methods", allowedCORSMethods)
			w.Header().Set("Access-Control-Allow-Headers", allowedCORSHeaders)
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			if r.Method == http.MethodOptions {
				if !validCORSPreflight(r) {
					writeError(w, r, http.StatusForbidden, "preflight not allowed")
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func validCORSPreflight(r *http.Request) bool {
	method := strings.ToUpper(strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")))
	if method == "" {
		return true
	}
	if _, ok := allowedCORSMethodSet[method]; !ok {
		return false
	}

	for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
		header = strings.ToLower(strings.TrimSpace(header))
		if header == "" {
			continue
		}
		if _, ok := allowedCORSHeaderSet[header]; !ok {
			return false
		}
	}
	return true
}

func normalizeOrigins(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func allowedCORSOrigin(allowedOrigins []string, origin string) (string, bool) {
	for _, allowed := range allowedOrigins {
		if allowed == "*" {
			return "*", true
		}
		if origin != "" && strings.EqualFold(allowed, origin) {
			return origin, true
		}
	}
	return "", false
}

func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf[:])
}
