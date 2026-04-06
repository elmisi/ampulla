package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elmisi/ampulla-mcp/internal/client"
	"github.com/elmisi/ampulla-mcp/internal/tools"
)

func main() {
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
