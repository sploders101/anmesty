package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"charm.land/log/v2"
	"github.com/urfave/cli/v3"
)

func main() {
	handler := log.New(os.Stderr)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if ipcFdStr := os.Getenv("AMNESTY_IPC_FD"); ipcFdStr != "" {
		// This isn't a CLI. Start the build API service.
		ipcFd, err := strconv.Atoi(ipcFdStr)
		if err != nil {
			slog.Error("")
		}

		runBuildService(ipcFd)

		os.Exit(0)
		return
	}

	cmd := &cli.Command{
		Commands: []*cli.Command{
			installCli(),
		},
	}
	if err := cmd.Run(ctx, os.Args); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
