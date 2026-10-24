package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yungbluth/streetworks/internal/store"
)

const (
	maxLookupIDs = 2000
	defaultLimit = 20
	maxLimit     = 50
	maxBodyBytes = 64 << 10
	maxResponse  = 2 << 20
)

type Store interface {
	Meta(ctx context.Context) (store.Meta, error)
	Lookup(ctx context.Context, ids []int) (map[int]store.Detail, error)
	ByNumber(ctx context.Context, number string) (store.Permit, error)
	Search(ctx context.Context, q string, limit int) ([]store.Hit, error)
}

type Options struct {
	// Origin is the one site allowed to call the API from a browser
	Origin string
	Log    *slog.Logger
	// rate and Burst set the per-client request limit, in requests per second and requests at once
	Rate  float64
	Burst int
	Now   func() time.Time
}

type server struct {
	store Store
	built string
	log   *slog.Logger
}

// New returns the API's handler
func New(s Store, built string, o Options) http.Handler {
	log := o.Log
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	rate, burst, now := o.Rate, o.Burst, o.Now
	if rate == 0 {
		rate = defaultRate
	}
	if burst == 0 {
		burst = defaultBurst
	}
	if now == nil {
		now = time.Now
	}
	// TODO limits per route? probably overkill for this
	lim := newLimiter(rate, burst, now)
	srv := &server{store: s, built: built, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", allow("GET", srv.healthz))
	mux.HandleFunc("/v1/meta", allow("GET", srv.meta))
	mux.HandleFunc("/v1/permits/lookup", allow("POST", srv.lookup))
	mux.HandleFunc("/v1/permits/{number}", allow("GET", srv.byNumber))
	mux.HandleFunc("/v1/search", allow("GET", srv.search))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "There is nothing at this address.")
	})

	// builtHeader stays outermost so that a 429 carries the build too
	return builtHeader(built, logRequests(log, lim.middleware(cors(o.Origin, gzipResponses(srv.stamp(mux))))))
}

func allow(method string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeError(w, http.StatusMethodNotAllowed, "This address only answers "+method+" requests.")
			return
		}
		h(w, r)
	}
}

func builtHeader(built string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Streetworks-Built", built)
		next.ServeHTTP(w, r)
	})
}

// stamp adds the cache headers to GETs, answering 304 when the client already holds this build
func (s *server) stamp(next http.Handler) http.Handler {
	etag := `"` + s.built + `"`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path != "/healthz" {
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "public, max-age=3600")
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func cors(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if o := r.Header.Get("Origin"); o != "" && allowedOrigin(origin, o) {
			w.Header().Set("Access-Control-Allow-Origin", o)
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedOrigin(configured, got string) bool {
	if configured != "" {
		return got == configured
	}
	u, err := url.Parse(got)
	return err == nil && u.Scheme == "http" && u.Hostname() == "localhost"
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "ok")
}

func (s *server) meta(w http.ResponseWriter, r *http.Request) {
	m, err := s.store.Meta(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, m)
}

func (s *server) lookup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int `json:"ids"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusBadRequest, "That request is too large.")
			return
		}
		writeError(w, http.StatusBadRequest, `Send a JSON body like {"ids":[1,2,3]}.`)
		return
	}
	if len(body.IDs) > maxLookupIDs {
		writeError(w, http.StatusBadRequest, "Ask for at most 2000 permits at a time.")
		return
	}
	ids := make([]int, 0, len(body.IDs))
	seen := make(map[int]bool, len(body.IDs))
	for _, id := range body.IDs {
		if id < 0 {
			writeError(w, http.StatusBadRequest, "Permit ids can't be negative.")
			return
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	permits, err := s.store.Lookup(r.Context(), ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if permits == nil {
		permits = map[int]store.Detail{}
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{"built": s.built, "permits": permits})
}

func (s *server) byNumber(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	p, err := s.store.ByNumber(r.Context(), number)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound,
			"No permit "+strings.ToUpper(strings.TrimSpace(number))+" is on record.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{
		"built":  s.built,
		"id":     p.ID,
		"place":  p.Place,
		"permit": p.Detail,
	})
}

func (s *server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := defaultLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "The limit must be a whole number.")
			return
		}
		if n > 0 {
			limit = min(n, maxLimit)
		}
	}
	hits, err := s.store.Search(r.Context(), q.Get("q"), limit)
	if errors.Is(err, store.ErrBadQuery) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if hits == nil {
		hits = []store.Hit{}
	}
	s.writeJSON(w, r, http.StatusOK, map[string]any{"built": s.built, "hits": hits})
}

func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "Something went wrong on the server. Try again in a moment.")
}

func (s *server) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	body, err := encode(v)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(body) > maxResponse {
		writeError(w, http.StatusBadRequest, "That's too many permits for one request; ask for fewer.")
		return
	}
	sendJSON(w, status, body)
}

// encode leaves <, > and & alone so that search snippets keep their <mark> tags readable
func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sendJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(body)
}

// writeError sends {"message": msg}. Errors aren't worth caching
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Del("Cache-Control")
	w.Header().Del("ETag")
	body, _ := encode(map[string]string{"message": msg}) // a string map always encodes
	sendJSON(w, status, body)
}
