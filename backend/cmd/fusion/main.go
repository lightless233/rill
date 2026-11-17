package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/0x2E/fusion/internal/buildinfo"
	"github.com/0x2E/fusion/internal/config"
	"github.com/0x2E/fusion/internal/eventhub"
	"github.com/0x2E/fusion/internal/handler"
	"github.com/0x2E/fusion/internal/pull"
	"github.com/0x2E/fusion/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-isatty"
	"golang.org/x/sync/errgroup"
)

func main() {
	showVersion := flag.Bool("version", false, "Print version and commit, then exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("Rill %s (%s)\n", buildinfo.Version, buildinfo.Commit)
		return
	}

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	setupLogger(cfg)
	gin.SetMode(gin.ReleaseMode)

	st, err := store.New(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	hub := eventhub.New()
	defer hub.Close()

	puller := pull.NewWithContext(sigCtx, st, cfg)
	puller.SetFeedsChanged(hub.Notify)

	h, err := handler.New(st, cfg, puller)
	if err != nil {
		return err
	}
	h.SetEventHub(hub)
	r := h.SetupRouter()

	addr := net.JoinHostPort(cfg.ListenHost, strconv.Itoa(cfg.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	g, ctx := errgroup.WithContext(sigCtx)

	g.Go(func() error {
		slog.Info("starting server", "address", addr, "version", buildinfo.Version, "commit", buildinfo.Commit)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		if err := puller.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		slog.Info("shutting down")

		// Event streams are long-lived; close their subscriber channels before
		// asking the HTTP server to drain otherwise active connections.
		hub.Close()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("failed to shutdown server", "error", err)
		}

		return nil
	})

	return g.Wait()
}

func setupLogger(cfg *config.Config) {
	var level slog.Level
	switch cfg.LogLevel {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	switch cfg.LogFormat {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	case "auto":
		if isatty.IsTerminal(os.Stdout.Fd()) {
			handler = slog.NewTextHandler(os.Stdout, opts)
		} else {
			handler = slog.NewJSONHandler(os.Stdout, opts)
		}
	default:
		if isatty.IsTerminal(os.Stdout.Fd()) {
			handler = slog.NewTextHandler(os.Stdout, opts)
		} else {
			handler = slog.NewJSONHandler(os.Stdout, opts)
		}
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
