package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
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

	switch *transport {
	case "stdio":
		runStdio(ampullaURL)
	case "http":
		// HTTP mode does not use AMPULLA_TOKEN/USER/PASSWORD: each request must
		// carry its own Bearer token, which the MCP server validates against
		// Ampulla and forwards on the caller's behalf.
		if err := httpTransport(*httpAddr, ampullaURL); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "invalid transport: %q (must be stdio or http)\n", *transport)
		os.Exit(2)
	}
}

// runStdio constructs a single shared client (using AMPULLA_TOKEN or
// AMPULLA_USER/AMPULLA_PASSWORD) and runs the MCP server on stdin/stdout.
func runStdio(ampullaURL string) {
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

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ampulla-mcp",
		Version: "0.1.0",
	}, nil)
	tools.Register(server, c)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
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
