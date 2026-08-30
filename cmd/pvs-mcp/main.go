package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dangogh/pvs-monitoring/config"
	"github.com/dangogh/pvs-monitoring/pvs"
)

// defaultAPIURL is the pvs-api serving the array. pvs-mcp is spawned by the
// MCP client, which is not necessarily the machine holding the database, so it
// reads over HTTP rather than opening SQLite. Reading a local copy of the
// database would answer every question confidently from whenever that copy was
// last written.
const defaultAPIURL = "http://solar.local"

func main() {
	if err := run(os.Args[1:], os.Stderr, &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "pvs-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, logOut io.Writer, transport mcp.Transport) error {
	fs := flag.NewFlagSet("pvs-mcp", flag.ContinueOnError)
	var cfgPath, apiURL string
	var verbose bool
	fs.StringVar(&cfgPath, "config", config.DefaultPath(), "path to config file")
	fs.StringVar(&apiURL, "api", defaultAPIURL, "base URL of the pvs-api server")
	fs.BoolVar(&verbose, "verbose", false, "enable debug logging")
	fs.BoolVar(&verbose, "v", false, "enable debug logging (shorthand)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: level}))

	// The API is deliberately not probed here. A client launching while the
	// monitoring host is rebooting should still come up; the first tool call
	// reports the problem, and reports it as an outage rather than as data.
	api := pvs.NewClient(apiURL)
	logger.Info("pvs-mcp starting", "api", apiURL)

	server := mcp.NewServer(&mcp.Implementation{Name: "pvs-mcp", Version: "0.1.0"}, nil)
	pvs.RegisterTools(server, api, cfg)

	return server.Run(context.Background(), transport)
}
