package ai

import (
	"context"
	"net"
	"net/http"
	"os"
)

const aiSocketEnv = "HN_CRITIQUE_AI_SOCKET"

func newHTTPClient() *http.Client {
	socketPath := os.Getenv(aiSocketEnv)
	if socketPath == "" {
		return &http.Client{Timeout: httpTimeout}
	}

	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{
		Timeout:   httpTimeout,
		Transport: transport,
	}
}
