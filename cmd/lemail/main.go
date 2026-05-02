package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PikachuCN/LeMail/internal/auth"
	"github.com/PikachuCN/LeMail/internal/codeextract"
	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/frontend"
	"github.com/PikachuCN/LeMail/internal/httpapi"
	"github.com/PikachuCN/LeMail/internal/mailstore"
	"github.com/PikachuCN/LeMail/internal/realtime"
	"github.com/PikachuCN/LeMail/internal/smtpdebug"
	"github.com/PikachuCN/LeMail/internal/smtpserver"
	"github.com/PikachuCN/LeMail/internal/webhook"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	manager, err := config.LoadManager(config.DefaultPath())
	if err != nil {
		return err
	}
	store := mailstore.New()
	codeStore := codeextract.NewStore()
	smtpDebug := smtpdebug.New(smtpdebug.DefaultLimit)
	hub := realtime.NewHub()
	sessions := auth.NewSessionManager()
	webhooks := webhook.NewDispatcher(manager, logger)
	codes := codeextract.NewProcessor(manager, codeStore, logger)
	stopJanitor := make(chan struct{})
	go store.StartJanitor(stopJanitor, func() time.Duration { return manager.Get().RetentionDuration() })
	go codeStore.StartJanitor(stopJanitor, func() time.Duration { return manager.Get().RetentionDuration() })
	defer close(stopJanitor)

	smtp := smtpserver.NewWithDebug(manager, store, hub, logger, func(msg mailstore.Message) {
		codes.HandleMessage(msg)
		webhooks.HandleMessage(msg)
	}, smtpDebug)
	go func() {
		if err := smtp.ListenAndServe(); err != nil {
			logger.Error("smtp server failed", "error", err)
		}
	}()

	api := httpapi.NewWithCodesAndDebug(manager, store, hub, sessions, codeStore, smtpDebug, frontend.FS())
	cfg := manager.Get()
	httpServer := &http.Server{Addr: cfg.Server.HTTPAddr, Handler: api.Routes(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("http server listening", "addr", cfg.Server.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	return httpServer.Shutdown(shutdownCtx)
}
