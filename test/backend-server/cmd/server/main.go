package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pseudonator/recaptcha-test-backend-server/pkg/server"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func main() {
	os.Exit(start())
}

func start() int {
	log, err := createLogger()
	if err != nil {
		fmt.Println("Error setting up the logger:", err)
		return 1
	}

	defer func() {
		// If we cannot sync, there's probably something wrong with outputting logs,
		// so we probably cannot write using fmt.Println either.
		// Hence, ignoring the error for now.
		_ = log.Sync()
	}()

	s := server.New(server.Options{
		Log: log,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		if err := s.Start(); err != nil {
			log.Info("Error starting server", zap.Error(err))
			return err
		}
		return nil
	})

	<-ctx.Done()

	eg.Go(func() error {
		if err := s.Stop(); err != nil {
			log.Info("Error stopping server", zap.Error(err))
			return err
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return 1
	}
	return 0
}

func createLogger() (*zap.Logger, error) {
	return zap.NewProduction()
}
