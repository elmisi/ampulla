package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elmisi/ampulla-mcp/internal/client"
	"github.com/elmisi/ampulla-mcp/internal/tools"
)

func main() {
	transport := flag.String("transport", "stdio", "transport mode: stdio or http")
	httpAddr := flag.String("http-addr", "127.0.0.1:8765", "address to listen on (http transport only)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	ampullaURL := requireEnv("AMPULLA_URL")

	var c *client.Client
	var err error
	if token := os.Getenv("AMPULLA_TOKEN"); token != "" {
		c, err = client.NewWithToken(ampullaURL, token)
		if err != nil {
			slog.Error("failed to create client", "error", err)
			os.Exit(1)
		}
		slog.Debug("using Bearer token authentication")
	} else {
		user := requireEnv("AMPULLA_USER")
		password := requireEnv("AMPULLA_PASSWORD")
		c, err = client.New(ampullaURL, user, password)
		if err != nil {
			slog.Error("failed to create client", "error", err)
			os.Exit(1)
		}
		if err := c.Login(context.Background()); err != nil {
			slog.Error("initial login failed", "error", err)
			os.Exit(1)
		}
	}

	makeServer := func() *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{
			Name:    "ampulla-mcp",
			Version: "0.1.0",
		}, nil)
		tools.Register(s, c)
		return s
	}

	switch *transport {
	case "stdio":
		if err := makeServer().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	case "http":
		handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
			return makeServer()
		}, nil)
		slog.Info("MCP HTTP server listening", "addr", *httpAddr)
		srv := &http.Server{Addr: *httpAddr, Handler: handler}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "invalid transport: %q (must be stdio or http)\n", *transport)
		os.Exit(2)
	}
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "required environment variable %s is not set\n", key)
		os.Exit(1)
	}
	return v
}
