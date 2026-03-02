package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"os/signal"
	"syscall"
	"time"
	"io"

	"reserveos/internal/reserveconfig"
	"reserveos/internal/reservewebfs"
)

type ServiceSpec struct {
	Name string `json:"name"`
	Enabled bool `json:"enabled"`
	Command []string `json:"command"` // ["go","run","./core/cmd/node","config/node/node.json"]
	WorkDir string `json:"workdir"`
	Env map[string]string `json:"env"`
}

type RouteSpec struct {
	PathPrefix string `json:"path_prefix"` // "/api"
	Target string `json:"target"` // "http://127.0.0.1:18445"
}

type WebSpec struct {
	RuntimeWebDir string `json:"runtime_web_dir"`
	MarketingBundle reservewebfs.Bundle `json:"marketing"`
	PortalBundle reservewebfs.Bundle `json:"portal"`
	WalletBundle reservewebfs.Bundle `json:"wallet"`
}

type Config struct {
	TCP struct{ Enabled bool `json:"enabled"`; Listen string `json:"listen"`; Target string `json:"target"` } `json:"tcp"`

	Security struct{
		PlatformDBTokenEnv string `json:"platformdb_token_env"` // env var containing platformdb token
	} `json:"security"`

	Gateway struct {
		Bind string `json:"bind"`
		Port int `json:"port"`
	} `json:"gateway"`

	Web WebSpec `json:"web"`

	Routes []RouteSpec `json:"routes"`

	Services []ServiceSpec `json:"services"`
}

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Println("usage: initialize-daemon <config.json>")
		os.Exit(2)
	}
	cfgPath := flag.Arg(0)
	var cfg Config
	if err := reserveconfig.LoadJSON(cfgPath, &cfg); err != nil { log.Fatal(err) }

	if cfg.Security.PlatformDBTokenEnv == "" { cfg.Security.PlatformDBTokenEnv = "RESERVEOS_PLATFORMDB_TOKEN" }
	if cfg.Gateway.Bind == "" { cfg.Gateway.Bind = "0.0.0.0" }
	if cfg.Gateway.Port == 0 { cfg.Gateway.Port = 8080 }
	if cfg.TCP.Listen == "" { cfg.TCP.Listen = "0.0.0.0:18444" }
	if cfg.TCP.Target == "" { cfg.TCP.Target = "127.0.0.1:18444" }

	// Build flat runtime/web from configured bundles
	if cfg.Web.RuntimeWebDir == "" { cfg.Web.RuntimeWebDir = filepath.Join("runtime","web") }
	bundles := []reservewebfs.Bundle{
		cfg.Web.MarketingBundle,
		cfg.Web.PortalBundle,
		cfg.Web.WalletBundle,
	}
	if err := reservewebfs.EnsureFlatWebFS(reservewebfs.Config{RuntimeWebDir: cfg.Web.RuntimeWebDir, Bundles: bundles}); err != nil {
		log.Fatal("webfs:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var procs []*exec.Cmd
	var mu sync.Mutex

	// Start enabled services
	for _, s := range cfg.Services {
		if !s.Enabled { continue }
		if len(s.Command) == 0 { continue }
		cmd := exec.CommandContext(ctx, s.Command[0], s.Command[1:]...)
		if s.WorkDir != "" { cmd.Dir = s.WorkDir }
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		env := os.Environ()
		for k, v := range s.Env { env = append(env, k+"="+v) }
		cmd.Env = env
		log.Println("starting", s.Name, strings.Join(s.Command, " "))
		if err := cmd.Start(); err != nil { log.Fatal(err) }
		mu.Lock(); procs = append(procs, cmd); mu.Unlock()
	}

	if cfg.TCP.Enabled {
		_ = startTCPProxy(cfg.TCP.Listen, cfg.TCP.Target)
		log.Println("tcp proxy", cfg.TCP.Listen, "->", cfg.TCP.Target)
	}

	// Reverse proxy mux
	mux := http.NewServeMux()
	lim := newRateLimiter(20, 40) // 20 rps, burst 40 per IP


	fetchJSON := func(url string) any {
		hc := &http.Client{Timeout: 2 * time.Second}
		resp, err := hc.Get(url)
		if err != nil { return map[string]any{"error": err.Error()} }
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var out any
		if err := json.Unmarshal(b, &out); err != nil {
			return map[string]any{"status": resp.StatusCode, "raw": string(b)}
		}
		return out
	}


	// Health
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })


	// Gateway API surface


	mux.HandleFunc("/api/network/peers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type","application/json")
		nodeBase := "http://127.0.0.1:18445"
		_ = json.NewEncoder(w).Encode(fetchJSON(nodeBase + "/peers/list"))
	})

	mux.HandleFunc("/api/network/peers/add", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		nodeBase := "http://127.0.0.1:18445"
		// proxy body through
		hc := &http.Client{Timeout: 3 * time.Second}
		resp, err := hc.Post(nodeBase + "/peers/add", "application/json", r.Body)
		if err != nil { w.WriteHeader(502); _ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()}); return }
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		w.WriteHeader(resp.StatusCode)
		w.Write(b)
	})

	
	mux.HandleFunc("/api/network/metrics", func(w http.ResponseWriter, r *http.Request) {
		nodeBase := "http://127.0.0.1:18445"
		hc := &http.Client{Timeout: 2 * time.Second}
		resp, err := hc.Get(nodeBase + "/metrics")
		if err != nil { w.WriteHeader(502); w.Write([]byte("upstream error")); return }
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		w.Header().Set("Content-Type","text/plain")
		w.WriteHeader(resp.StatusCode)
		w.Write(b)
	})
mux.HandleFunc("/api/network/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type","application/json")
		// These upstreams assume default route targets in config
		nodeBase := "http://127.0.0.1:18445"
		status := map[string]any{
			"node": map[string]any{
				"chain": fetchJSON(nodeBase + "/chain/info"),
				"mempool": fetchJSON(nodeBase + "/mempool/info"),
				"gossip": fetchJSON(nodeBase + "/gossip/info"),
				"p2p": fetchJSON(nodeBase + "/p2p/info"),
				"fees": fetchJSON(nodeBase + "/fees/info"),
			},
		}
		_ = json.NewEncoder(w).Encode(status)
	})

	
	mux.HandleFunc("/api/gateway/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type","application/json")
		fetch := func(url string) bool {
			hc := &http.Client{Timeout: 1 * time.Second}
			resp, err := hc.Get(url)
			if err != nil { return false }
			defer resp.Body.Close()
			return resp.StatusCode >= 200 && resp.StatusCode < 500
		}
		st := map[string]any{
			"ok": true,
			"p2p_tcp": (func() bool { c, err := net.DialTimeout("tcp", "127.0.0.1:18444", 500*time.Millisecond); if err != nil { return false }; c.Close(); return true })(),
			"node_http": fetch("http://127.0.0.1:18445/healthz"),
			"wallet_http": fetch("http://127.0.0.1:9020/healthz"),
			"platformdb_http": fetch("http://127.0.0.1:9030/healthz"),
		}
		_ = json.NewEncoder(w).Encode(st)
	})
mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"p2p_tcp": (func() bool { c, err := net.DialTimeout("tcp", "127.0.0.1:18444", 500*time.Millisecond); if err != nil { return false }; c.Close(); return true })(),
			"gateway": map[string]any{"addr": addr},
		})
	})

	// Web routes (flat webfs)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		// serve exact routes
		switch {
		case p == "/" || p == "":
			http.ServeFile(w, r, filepath.Join(cfg.Web.RuntimeWebDir, "marketing.index.html"))
		case strings.HasPrefix(p, "/portal"):
			http.ServeFile(w, r, filepath.Join(cfg.Web.RuntimeWebDir, "portal.index.html"))
		case strings.HasPrefix(p, "/wallet"):
			http.ServeFile(w, r, filepath.Join(cfg.Web.RuntimeWebDir, "wallet.index.html"))
		default:
			// serve flat assets if present
			f := strings.TrimPrefix(p, "/")
			try := filepath.Join(cfg.Web.RuntimeWebDir, f)
			if _, err := os.Stat(try); err == nil {
				http.ServeFile(w, r, try)
				return
			}
			http.NotFound(w, r)
		}
	})

	// API proxy routes
	for _, rt := range cfg.Routes {
		rt := rt
		tgt, err := url.Parse(rt.Target)
		if err != nil { log.Fatal(err) }
		proxy := httputil.NewSingleHostReverseProxy(tgt)
		mux.Handle(rt.PathPrefix + "/", http.StripPrefix(rt.PathPrefix, proxy))
		mux.Handle(rt.PathPrefix, http.StripPrefix(rt.PathPrefix, proxy))
	}

	addr := fmt.Sprintf("%s:%d", cfg.Gateway.Bind, cfg.Gateway.Port)
	log.Println("gateway listening on", addr)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/healthz" && r.URL.Path != "/api/network/status" {
			// basic request size cap (2MB)
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
			ip := realIP(r)
			if !lim.allow(ip) {
				w.WriteHeader(429)
				w.Write([]byte("rate limited"))
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
	srv := &http.Server{Addr: addr, Handler: handler, ReadTimeout: 10*time.Second, WriteTimeout: 10*time.Second, IdleTimeout: 30*time.Second}
	go func() {
		_ = srv.ListenAndServe()
	}()

	// wait until killed
	waitCh := make(chan os.Signal, 1)
	signal.Notify(waitCh, os.Interrupt, syscall.SIGTERM)
	<-waitCh

	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	_ = srv.Shutdown(ctx2)
	cancel()
	mu.Lock()
	for _, p := range procs { _ = p.Process.Kill() }
	mu.Unlock()
}




func startTCPProxy(listenAddr, targetAddr string) error {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil { return err }
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil { return }
			go func() {
				defer c.Close()
				t, err := net.Dial("tcp", targetAddr)
				if err != nil { return }
				defer t.Close()
				go io.Copy(t, c)
				io.Copy(c, t)
			}()
		}
	}()
	return nil
}



type rateLimiter struct {
	mu sync.Mutex
	m map[string]struct{ tokens float64; last int64 }
	rate float64
	burst float64
}

func newRateLimiter(rate, burst float64) *rateLimiter {
	return &rateLimiter{m: map[string]struct{ tokens float64; last int64 }{}, rate: rate, burst: burst}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now().UnixNano()
	rec, ok := rl.m[ip]
	if !ok { rec.tokens = rl.burst; rec.last = now }
	dt := float64(now - rec.last) / 1e9
	rec.tokens = minf(rl.burst, rec.tokens + dt*rl.rate)
	rec.last = now
	if rec.tokens < 1.0 {
		rl.m[ip] = rec
		return false
	}
	rec.tokens -= 1.0
	rl.m[ip] = rec
	return true
}

func minf(a,b float64) float64 { if a<b { return a }; return b }


func realIP(r *http.Request) string {
	// naive: if X-Forwarded-For set, use first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 { return strings.TrimSpace(parts[0]) }
	}
	rip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i != -1 { ip = ip[:i] }
	return ip
}
