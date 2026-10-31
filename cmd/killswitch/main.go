package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type config struct {
	Project     string
	MetadataURL string
	BillingURL  string
	Client      *http.Client
	Log         *slog.Logger
}

// pushBody is the envelope Pub/Sub wraps around every push delivery
type pushBody struct {
	Message struct {
		Data      string `json:"data"`
		MessageID string `json:"messageId"`
	} `json:"message"`
}

type budgetNotice struct {
	CostAmount        float64 `json:"costAmount"`
	BudgetAmount      float64 `json:"budgetAmount"`
	CurrencyCode      string  `json:"currencyCode"`
	BudgetDisplayName string  `json:"budgetDisplayName"`
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	project := os.Getenv("PROJECT_ID")
	if project == "" {
		fmt.Fprintln(os.Stderr, "killswitch: PROJECT_ID is not set")
		os.Exit(1)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr: ":" + port,
		Handler: handler(config{
			Project:     project,
			MetadataURL: "http://metadata.google.internal",
			BillingURL:  "https://cloudbilling.googleapis.com",
			Client:      &http.Client{Timeout: 10 * time.Second},
			Log:         log,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Info("listening", "addr", srv.Addr, "project", project)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// handler takes the pub/sub push and detaches billing if the budget shows a cost
func handler(cfg config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notice, err := readNotice(r.Body)
		if err != nil {
			cfg.Log.Warn("unreadable message", "error", err)
			http.Error(w, "bad message", http.StatusBadRequest)
			return
		}
		if notice.CostAmount <= 0 {
			cfg.Log.Info("no cost reported", "budget", notice.BudgetDisplayName)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		cfg.Log.Warn("cost reported, detaching billing",
			"cost", notice.CostAmount, "budget_amount", notice.BudgetAmount,
			"currency", notice.CurrencyCode, "budget", notice.BudgetDisplayName)
		if err := detach(r.Context(), cfg); err != nil {
			cfg.Log.Error("could not detach billing", "error", err)
			http.Error(w, "could not detach billing", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// readNotice decodes a push body
func readNotice(body io.Reader) (budgetNotice, error) {
	var push pushBody
	if err := json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&push); err != nil {
		return budgetNotice{}, fmt.Errorf("decoding push body: %w", err)
	}
	raw, err := base64.StdEncoding.DecodeString(push.Message.Data)
	if err != nil {
		return budgetNotice{}, fmt.Errorf("decoding message data: %w", err)
	}
	var n budgetNotice
	if err := json.Unmarshal(raw, &n); err != nil {
		return budgetNotice{}, fmt.Errorf("decoding budget notice: %w", err)
	}
	return n, nil
}

func detach(ctx context.Context, cfg config) error {
	token, err := accessToken(ctx, cfg)
	if err != nil {
		return err
	}
	url := cfg.BillingURL + "/v1/projects/" + cfg.Project + "/billingInfo"

	var info struct {
		BillingEnabled bool `json:"billingEnabled"`
	}
	if err := call(ctx, cfg, http.MethodGet, url, token, nil, &info); err != nil {
		return fmt.Errorf("reading billing info: %w", err)
	}
	if !info.BillingEnabled {
		cfg.Log.Info("billing is already detached", "project", cfg.Project)
		return nil
	}

	// An empty billingAccountName is how the API spells "detach"
	body := []byte(`{"billingAccountName":""}`)
	if err := call(ctx, cfg, http.MethodPut, url, token, body, nil); err != nil {
		return fmt.Errorf("detaching billing: %w", err)
	}
	cfg.Log.Warn("billing detached", "project", cfg.Project)
	return nil
}

func accessToken(ctx context.Context, cfg config) (string, error) {
	url := cfg.MetadataURL + "/computeMetadata/v1/instance/service-accounts/default/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := do(cfg, req, &tok); err != nil {
		return "", fmt.Errorf("getting access token: %w", err)
	}
	if tok.AccessToken == "" {
		return "", errors.New("getting access token: metadata server returned none")
	}
	return tok.AccessToken, nil
}

func call(ctx context.Context, cfg config, method, url, token string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return do(cfg, req, out)
}

func do(cfg config, req *http.Request, out any) error {
	resp, err := cfg.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: %s: %s", req.Method, req.URL.Path, resp.Status, bytes.TrimSpace(msg))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
