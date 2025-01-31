package main

import (
	"context"
	"embed"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/yarlson/duh/docker"
	"github.com/yarlson/duh/logger"
	"github.com/yarlson/duh/server"
	"github.com/yarlson/duh/service"
	"github.com/yarlson/duh/store"
)

//go:embed www/dist
var StaticFiles embed.FS

const (
	serverPort = ":4242"
	serverURL  = "http://localhost" + serverPort
)

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func main() {
	l := logger.New()
	l.Info("Starting duh...")
	dockerClient := docker.NewClient()
	memoryStore := store.NewStore(30 * time.Second)
	containerService := service.New(dockerClient, memoryStore)

	if err := containerService.Sync(context.Background()); err != nil {
		l.Fatal("Initial sync failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := containerService.Sync(ctx); err != nil {
					l.Warn("Sync error: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	srv := server.New(containerService, StaticFiles)

	go func() {
		if err := srv.ListenAndServe(serverPort); err != nil {
			l.Fatal("Server failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)
	if err := openBrowser(serverURL); err != nil {
		l.Warn("Failed to open browser: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	l.Info("Shutting down...")
	cancel()
	memoryStore.Close()
	l.Info("Server stopped gracefully")
}
