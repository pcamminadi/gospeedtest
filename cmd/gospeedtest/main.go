// gospeedtest is the CLI entry point.
//
// Usage:
//
//	gospeedtest                                     # CLI mode (default)
//	gospeedtest [--server URL] [--json] ...         # CLI mode with flags
//	gospeedtest cli    [--server URL] ...           # CLI mode (explicit)
//	gospeedtest server [--addr :8080] ...           # run the server
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/pcamminadi/gospeedtest/internal/cli"
	"github.com/pcamminadi/gospeedtest/internal/server"
)

func main() {
	args := os.Args[1:]

	// Dispatch on the first positional arg if it names a subcommand. Anything
	// else (no args, or args that start with a flag) falls through to CLI
	// mode — that's the most common invocation, so it gets the shortest path.
	if len(args) > 0 {
		switch args[0] {
		case "server":
			runServer(args[1:])
			return
		case "cli":
			runCLI(args[1:])
			return
		case "version", "--version", "-v":
			fmt.Println(version())
			return
		case "help", "--help", "-h":
			usage()
			return
		}
	}
	runCLI(args)
}

func usage() {
	fmt.Fprintf(os.Stderr, `gospeedtest %s — open-source self-hosted speed test

usage:
  gospeedtest                                     run a speed test (default)
  gospeedtest [--server URL] [--json] ...         run a speed test with flags
  gospeedtest cli    [--server URL] ...           run a speed test (explicit)
  gospeedtest server [--addr :8080] ...           run the server
  gospeedtest version

flags (cli — also the defaults when no subcommand is given):
  --server         server URL (default "http://localhost:8080")
  --json           emit a single JSON summary (skip the TUI)
  --duration       per-phase test duration (default 10s)
  --streams        parallel HTTP streams (default 4)

flags (server):
  --addr           listen address (default ":8080")
  --ipinfo-token   optional ipinfo.io token for richer ISP data
`, version())
}

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	token := fs.String("ipinfo-token", "", "optional ipinfo.io token")
	_ = fs.Parse(args)

	srv := server.New(server.Config{Addr: *addr, IPInfoToken: *token})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("gospeedtest server listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Print("shutting down…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func runCLI(args []string) {
	fs := flag.NewFlagSet("cli", flag.ExitOnError)
	url := fs.String("server", "http://localhost:8080", "server URL")
	jsonOut := fs.Bool("json", false, "emit a JSON summary instead of the TUI")
	dur := fs.Duration("duration", 10*time.Second, "per-phase test duration")
	streams := fs.Int("streams", 4, "parallel HTTP streams")
	_ = fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cli.Run(ctx, cli.Options{
		ServerURL: *url,
		JSON:      *jsonOut,
		Duration:  *dur,
		Streams:   *streams,
	}); err != nil {
		log.Fatalf("cli: %v", err)
	}
}

// versionTag is set at link time by release builds: -ldflags "-X main.versionTag=vX.Y.Z".
var versionTag string

// version returns the linked release tag if present, otherwise the module
// version reported by the build info, otherwise "dev".
func version() string {
	if versionTag != "" {
		return versionTag
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}
