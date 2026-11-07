package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gkuba/ReelAnvil/internal/store"
	"github.com/gkuba/ReelAnvil/internal/worker"
)

// workerTokenPath holds the shared secret the worker sends with each request.
const workerTokenPath = "settings/worker.token"

// ensureWorkerToken creates the token file once; later starts reuse it.
func ensureWorkerToken(dataDir string) (string, error) {
	p := filepath.Join(dataDir, workerTokenPath)
	if b, err := os.ReadFile(p); err == nil {
		if t := strings.TrimSpace(string(b)); len(t) >= 32 {
			return t, nil
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := hex.EncodeToString(b)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(t+"\n"), 0o600); err != nil {
		return "", err
	}
	return t, os.Rename(tmp, p)
}

// workerAuth accepts only loopback requests carrying the token.
func (s *Server) workerAuth(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if ip == nil || !ip.IsLoopback() || s.workerToken == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.workerToken)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) workerClaim(w http.ResponseWriter, r *http.Request) {
	op, ok, err := s.store.ClaimOp(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(worker.Op{ID: op.ID, Kind: op.Kind, Args: op.Args})
}

// workerOp handles /api/worker/ops/{id}/progress and /finish.
func (s *Server) workerOp(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/worker/ops/")
	idText, action, _ := strings.Cut(rest, "/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch action {
	case "progress":
		var p struct {
			Done, Total int64
			Detail      string
		}
		if json.Unmarshal(body, &p) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		err = s.store.OpProgress(r.Context(), id, p.Done, p.Total, clip(p.Detail, 200))
	case "finish":
		var f struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if json.Unmarshal(body, &f) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		err = s.store.FinishOp(r.Context(), id, f.OK, f.Result, clip(f.Error, 2000))
	default:
		http.NotFound(w, r)
		return
	}
	switch {
	case errors.Is(err, store.ErrOpCancelled):
		http.Error(w, "cancelled", http.StatusConflict)
	case errors.Is(err, store.ErrNotFound):
		http.NotFound(w, r)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// WorkerOptions are the "reelanvil worker" flags.
type WorkerOptions struct {
	DataDir string
	Server  string // host:port of the web app
}

// RunWorker runs the media worker until SIGINT or SIGTERM. It reads the
// server's settings and token from the data directory.
func RunWorker(opts WorkerOptions) error {
	dataDir, err := filepath.Abs(opts.DataDir)
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(opts.Server)
	if ip := net.ParseIP(host); err != nil || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
		return fmt.Errorf("--server must be a loopback host:port, such as 127.0.0.1:4747")
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dataDir, "logs/worker.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	logger := log.New(io.MultiWriter(file, os.Stderr), "", log.LstdFlags|log.LUTC)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The server writes the token on its first start; wait for it.
	var token string
	for {
		b, err := os.ReadFile(filepath.Join(dataDir, workerTokenPath))
		if token = strings.TrimSpace(string(b)); err == nil && token != "" {
			break
		}
		logger.Printf("waiting for %s (the web app creates it when it starts)", workerTokenPath)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second):
		}
	}
	cfg, err := load(filepath.Join(dataDir, configPath))
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	w, err := worker.New(worker.Config{Server: opts.Server, Token: token, Engine: cfg.MediaEngine, FFmpeg: cfg.FFmpegPath, YtDlp: cfg.YtDlpPath,
		MediaRoot: effectiveMediaRoot(cfg, dataDir), Log: logger})
	if err != nil {
		return err
	}
	return w.Run(ctx)
}
