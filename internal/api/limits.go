package api

import (
	"compress/gzip"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultRate  = 10.0
	defaultBurst = 20
	idleAfter    = time.Minute
)

// limiter is a token bucket per client
// TODO: all of this is in memory, so the limits only hold while Cloud Run max instances is 1
type limiter struct {
	rate  float64
	burst float64
	now   func() time.Time

	mu        sync.Mutex
	clients   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	seen   time.Time
}

func newLimiter(rate float64, burst int, now func() time.Time) *limiter {
	return &limiter{
		rate:      rate,
		burst:     float64(burst),
		now:       now,
		clients:   map[string]*bucket{},
		lastSweep: now(),
	}
}

// allow takes a token for key, or says how many seconds until there is one
func (l *limiter) allow(key string) (ok bool, retryAfter int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if now.Sub(l.lastSweep) >= idleAfter {
		for k, b := range l.clients {
			if now.Sub(b.seen) > idleAfter {
				delete(l.clients, k)
			}
		}
		l.lastSweep = now
	}

	b, found := l.clients[key]
	if !found {
		b = &bucket{tokens: l.burst, seen: now}
		l.clients[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.seen).Seconds()*l.rate)
	b.seen = now
	if b.tokens < 1 {
		return false, int(math.Ceil((1 - b.tokens) / l.rate))
	}
	b.tokens--
	return true, 0
}

func (l *limiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.clients)
}

// clientKey is the rightmost address in X-Forwarded-For that parses, falling back to the peer
func clientKey(r *http.Request) string {
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		if addr, err := netip.ParseAddr(strings.TrimSpace(hops[i])); err == nil {
			return addr.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *limiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, retry := l.allow(clientKey(r)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			writeError(w, http.StatusTooManyRequests, "Too many requests. Wait a moment and try again.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// logRequests writes one line per request
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		// the mux fills in r.Pattern on this same request
		route := r.Pattern
		if route == "" {
			route = r.URL.Path
		}
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		log.Info("request",
			"method", r.Method,
			"route", route,
			"status", status,
			"ms", time.Since(start).Milliseconds(),
			"bytes", rec.bytes,
		)
	})
}

// gzipper compresses a response when the client accepts it
type gzipper struct {
	http.ResponseWriter
	zw      *gzip.Writer
	status  int
	started bool
}

func (g *gzipper) WriteHeader(code int) {
	if !g.started && g.status == 0 {
		g.status = code
	}
}

func (g *gzipper) start() {
	g.started = true
	if g.status == 0 {
		g.status = http.StatusOK
	}
	if g.status != http.StatusNoContent && g.status != http.StatusNotModified {
		g.Header().Set("Content-Encoding", "gzip")
		g.Header().Del("Content-Length")
		g.zw = gzip.NewWriter(g.ResponseWriter)
	}
	g.ResponseWriter.WriteHeader(g.status)
}

func (g *gzipper) Write(p []byte) (int, error) {
	if !g.started {
		g.start()
	}
	if g.zw == nil {
		return g.ResponseWriter.Write(p)
	}
	return g.zw.Write(p)
}

func (g *gzipper) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func (g *gzipper) finish() {
	switch {
	case g.zw != nil:
		g.zw.Close()
	case !g.started && g.status != 0:
		g.ResponseWriter.WriteHeader(g.status)
	}
}

func gzipResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		g := &gzipper{ResponseWriter: w}
		defer g.finish()
		next.ServeHTTP(g, r)
	})
}

func acceptsGzip(r *http.Request) bool {
	for _, v := range r.Header.Values("Accept-Encoding") {
		for _, enc := range strings.Split(v, ",") {
			name, params, _ := strings.Cut(strings.TrimSpace(enc), ";")
			if strings.EqualFold(name, "gzip") && strings.ReplaceAll(strings.TrimSpace(params), " ", "") != "q=0" {
				return true
			}
		}
	}
	return false
}
