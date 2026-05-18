package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	appdb "voicescribe-webhook/internal/db"
	"voicescribe-webhook/internal/tts"
	"voicescribe-webhook/internal/users"
	"voicescribe-webhook/internal/webhook"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "Probe /healthz on $PORT and exit 0/1 (for container healthchecks)")
	flag.Parse()

	if *healthcheck {
		os.Exit(probeHealth(getPort()))
	}

	if err := run(); err != nil {
		slog.Error("voicescribe: fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	logger := newLogger()
	slog.SetDefault(logger)

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	port := getPort()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	conn, err := openWithRetry(ctx, dsn, 30*time.Second)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer conn.Close()

	tuneConnPool(conn)

	if err := appdb.Migrate(ctx, conn); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	
ttsClient, err := tts.NewClient(
os.Getenv("OPENROUTER_API_KEY"),
os.Getenv("STORAGE_ENDPOINT"),
os.Getenv("STORAGE_ACCESS_KEY"),
os.Getenv("STORAGE_SECRET_KEY"),
os.Getenv("STORAGE_BUCKET"),
os.Getenv("STORAGE_PUBLIC_URL"),
)
if err != nil {
return fmt.Errorf("init tts client: %w", err)
}

	slog.Info("newscriber: loading twilio & whatsapp environment configuration",
		"twilio_account_sid_present", os.Getenv("TWILIO_ACCOUNT_SID") != "",
		"twilio_auth_token_present", os.Getenv("TWILIO_AUTH_TOKEN") != "",
		"twilio_whatsapp_number", os.Getenv("TWILIO_WHATSAPP_NUMBER"),
		"meta_whatsapp_number", os.Getenv("META_WHATSAPP_NUMBER"),
	)

	repo := users.NewPgUserRepository(conn)
	handler := webhook.NewHandler(repo, ttsClient, conn)

	mux := http.NewServeMux()
	mux.Handle("/webhook/whatsapp", handler)
	mux.HandleFunc("/tts", handler.HandleTTS)
	mux.HandleFunc("/visualizer", handler.HandleVisualizer)
	mux.HandleFunc("/episodes", handler.HandleEpisodes)
	mux.HandleFunc("/api/episodes", handler.HandleEpisodes)
	mux.HandleFunc("/trigger", handler.HandleTrigger)
	mux.HandleFunc("/api/trigger", handler.HandleTrigger)
	mux.HandleFunc("/publish", handler.HandlePublish)
	mux.HandleFunc("/api/publish", handler.HandlePublish)
	mux.HandleFunc("/rss/generate", func(w http.ResponseWriter, r *http.Request) {
		for _, lang := range []string{"en", "it", "fr", "bn", "global"} {
			if err := handler.UpdateRSSFeed(r.Context(), lang); err != nil {
				slog.Error("failed manual RSS feed generation", "lang", lang, "err", err)
				http.Error(w, "failed for "+lang+": "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("all RSS feeds successfully generated and uploaded to R2!"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           requestLogger(logger, mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("voicescribe: listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("voicescribe: server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("voicescribe: shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func getPort() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8080"
}

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.EqualFold(os.Getenv("LOG_FORMAT"), "text") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

func tuneConnPool(conn *sql.DB) {
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)
	conn.SetConnMaxIdleTime(2 * time.Minute)
}

func openWithRetry(ctx context.Context, dsn string, timeout time.Duration) (*sql.DB, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := conn.PingContext(pingCtx)
		cancel()
		if err == nil {
			return conn, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			_ = conn.Close()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	_ = conn.Close()
	return nil, fmt.Errorf("ping db: %w", lastErr)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"dur_ms", time.Since(start).Milliseconds(),
		)
	})
}

func probeHealth(port string) int {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: time.Second}).DialContext,
		},
	}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}
