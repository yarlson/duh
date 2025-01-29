package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

//go:embed index.html
var content embed.FS

const (
	colorReset  = "\033[0m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[36m"
	colorGray   = "\033[90m"
	// OSC 8 escape sequence for clickable links
	linkStart = "\033]8;;"
	linkEnd   = "\033\\"
)

type colorLogger struct {
	*log.Logger
}

func newLogger() *colorLogger {
	flags := log.Lmsgprefix
	prefix := fmt.Sprintf("%sduh%s %s>%s ", colorBlue, colorReset, colorGray, colorReset)
	return &colorLogger{log.New(os.Stdout, prefix, flags)}
}

func (l *colorLogger) Info(format string, v ...interface{}) {
	l.Printf(format, v...)
}

func (l *colorLogger) Warn(format string, v ...interface{}) {
	l.Printf("%s%s%s", colorYellow, fmt.Sprintf(format, v...), colorReset)
}

func (l *colorLogger) Fatal(format string, v ...interface{}) {
	l.Logger.Fatalf(format, v...)
}

// Add helper method for clickable links
func (l *colorLogger) link(url string) string {
	return fmt.Sprintf("%s%s%s%s%s", linkStart, url, linkEnd, url, linkStart+linkEnd)
}

// openBrowser opens the default browser based on the OS
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func newDockerProxy() (*httputil.ReverseProxy, error) {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", "/var/run/docker.sock")
		},
	}

	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "docker"
			req.Host = "docker"
		},
		Transport: transport,
	}, nil
}

func main() {
	// Initialize logger
	logger := newLogger()

	// Create Docker socket proxy
	proxy, err := newDockerProxy()
	if err != nil {
		logger.Fatal("Failed to create Docker proxy: %v", err)
	}

	// Create server and configure routes
	mux := http.NewServeMux()

	// Serve main page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		tmpl, err := template.ParseFS(content, "index.html")
		if err != nil {
			logger.Info("Failed to parse template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if err := tmpl.Execute(w, nil); err != nil {
			logger.Info("Failed to execute template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	})

	// Proxy Docker API requests
	mux.HandleFunc("/docker/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = r.URL.Path[7:] // Remove "/docker" prefix
		proxy.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:         ":4242",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		url := fmt.Sprintf("http://localhost%s", srv.Addr)
		logger.Info("Starting server on %s", logger.link(url))
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("Server failed: %v", err)
		}
	}()

	// Open browser after a short delay to ensure server is ready
	time.Sleep(100 * time.Millisecond)
	if err := openBrowser("http://localhost:4242"); err != nil {
		logger.Warn("Failed to open browser: %v", err)
	}

	// Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown: %v", err)
	}

	logger.Info("Server stopped gracefully")
}
