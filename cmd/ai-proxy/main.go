// Command ai-proxy forwards AI requests over a local Unix socket, keeping
// secrets and outbound network access outside the analysis process.
package main

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

const (
	defaultProxySocket = "/tmp/hn-critique-ai.sock"
	defaultProxyTarget = "https://models.github.ai/inference"
	socketEnv          = "HN_CRITIQUE_AI_SOCKET"
	upstreamEnv        = "HN_CRITIQUE_AI_UPSTREAM"
	tokenEnv           = "HN_CRITIQUE_AI_TOKEN"
)

var allowedPaths = map[string]struct{}{
	"/chat/completions":    {},
	"/v1/chat/completions": {},
	"/v1/responses":        {},
}

func main() {
	socketPath := os.Getenv(socketEnv)
	if socketPath == "" {
		socketPath = defaultProxySocket
	}
	upstream := os.Getenv(upstreamEnv)
	if upstream == "" {
		upstream = defaultProxyTarget
	}
	token := os.Getenv(tokenEnv)
	if token == "" {
		log.Fatal("HN_CRITIQUE_AI_TOKEN must be set")
	}

	targetURL, err := url.Parse(upstream)
	if err != nil {
		log.Fatalf("Invalid upstream URL %q: %v", upstream, err)
	}

	if err := os.RemoveAll(socketPath); err != nil {
		log.Fatalf("Failed to remove existing socket %s: %v", socketPath, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = &http.Transport{}
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
		req.Header.Set("Authorization", "Bearer "+token)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, ok := allowedPaths[r.URL.Path]; !ok {
			http.Error(w, "path not allowed", http.StatusNotFound)
			return
		}
		proxy.ServeHTTP(w, r)
	})

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		log.Fatalf("Failed to chmod socket %s: %v", socketPath, err)
	}

	log.Printf("AI proxy listening on unix://%s forwarding to %s", socketPath, upstream)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("AI proxy server error: %v", err)
	}
}
