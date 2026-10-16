package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

// long: 前端随二进制一起发布，避免切换工作目录或更新时遗漏静态文件导致白屏。
//
//go:embed public/index.html public/app.js public/styles.css
var publicFiles embed.FS

type serverConfig struct {
	Host     string
	Port     string
	Interval time.Duration
	History  int
}

func loadConfig() (serverConfig, error) {
	cfg := serverConfig{Host: envString("HOST", defaultHost), Port: envString("PORT", defaultPort)}
	if _, err := boundedEnvInt("PORT", defaultPort, 1, 65535); err != nil {
		return cfg, err
	}
	interval, err := boundedEnvInt("SAMPLE_INTERVAL_MS", strconv.Itoa(defaultIntervalMs), 1000, 60000)
	if err != nil {
		return cfg, err
	}
	history, err := boundedEnvInt("MAX_HISTORY", "120", 1, 3600)
	if err != nil {
		return cfg, err
	}
	cfg.Interval = time.Duration(interval) * time.Millisecond
	cfg.History = history
	return cfg, nil
}

func boundedEnvInt(name, fallback string, minimum, maximum int) (int, error) {
	raw := envString(name, fallback)
	value, err := strconv.Atoi(raw)
	// long: 配置错误应阻止启动并在 journal 留下原因，不能静默回退到另一个端口。
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	return value, nil
}

func newHandler(state *AppState, interval time.Duration, shutdown context.Context) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"ok": true, "timestamp": time.Now().UTC().Format(time.RFC3339),
			"version": version, "commit": commit, "buildTime": buildTime,
		})
	})
	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, state.snapshot())
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		if _, err := fmt.Fprint(w, "retry: 2000\n\n"); err != nil {
			return
		}
		flusher.Flush()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-shutdown.Done():
				return
			case <-r.Context().Done():
				return
			case <-ticker.C:
				payload, err := json.Marshal(state.snapshot())
				if err != nil {
					return
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path
		if name == "/" {
			name = "/index.html"
		}
		// long: 只开放三个内嵌资源，禁止目录浏览，也不允许读取配置或服务器本地文件。
		switch name {
		case "/index.html", "/app.js", "/styles.css":
		default:
			http.NotFound(w, r)
			return
		}
		payload, err := publicFiles.ReadFile("public" + name)
		if err != nil {
			http.Error(w, "asset missing", http.StatusInternalServerError)
			return
		}
		switch name {
		case "/app.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case "/styles.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		_, _ = w.Write(payload)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("server-panel %s commit=%s buildTime=%s\n", version, commit, buildTime)
		return nil
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	state := &AppState{maxHistory: cfg.History}
	if err := state.collect(); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := state.collect(); err != nil {
					fmt.Fprintf(os.Stderr, "metrics collection failed: %v\n", err)
				}
			}
		}
	}()
	server := &http.Server{
		Addr: net.JoinHostPort(cfg.Host, cfg.Port), Handler: newHandler(state, cfg.Interval, ctx),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
	}
	shutdownResult := make(chan error, 1)
	go func() {
		<-ctx.Done()
		// long: 停止时先通知 SSE 退出，再等待 HTTP 请求结束，避免重启残留监听进程。
		timeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownResult <- server.Shutdown(timeout)
	}()
	fmt.Printf("Server Panel %s listening on http://%s\n", version, server.Addr)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return <-shutdownResult
	}
	return err
}
