package site

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func published(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/data/index.json"
}

func withCount(n int) string {
	return fmt.Sprintf(`{"version":1,"quality":{"unique_permits":%d}}`, n)
}

func TestGuard(t *testing.T) {
	ctx, c := context.Background(), http.DefaultClient
	url := published(t, 200, withCount(100000))

	if err := CheckGuard(ctx, c, url, 90000); err != nil {
		t.Errorf("a 10%% drop is allowed: %v", err)
	}
	if err := CheckGuard(ctx, c, url, 120000); err != nil {
		t.Errorf("growth is allowed: %v", err)
	}
	err := CheckGuard(ctx, c, url, 89999)
	if err == nil || !strings.Contains(err.Error(), "100000") || !strings.Contains(err.Error(), "89999") {
		t.Errorf("more than 10%% fewer should fail naming both counts, got %v", err)
	}
}

func TestGuardPassesOnFirstDeploy(t *testing.T) {
	if err := CheckGuard(context.Background(), http.DefaultClient, published(t, 404, "not found"), 5); err != nil {
		t.Errorf("nothing published yet: %v", err)
	}
	if err := CheckGuard(context.Background(), http.DefaultClient, "", 5); err != nil {
		t.Errorf("no URL means no guard: %v", err)
	}
}

func TestGuardFailsWhenItCannotTell(t *testing.T) {
	ctx, c := context.Background(), http.DefaultClient
	for name, url := range map[string]string{
		"malformed JSON": published(t, 200, "{nope"),
		"no count in it": published(t, 200, `{"version":1}`),
		"server error":   published(t, 500, "oops"),
		"unreachable":    "http://127.0.0.1:1/data/index.json",
	} {
		if err := CheckGuard(ctx, c, url, 100); err == nil {
			t.Errorf("%s: passed", name)
		}
	}
}
