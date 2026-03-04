package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/GURSEWAK13/claude-connector/internal/config"
	"github.com/GURSEWAK13/claude-connector/internal/fallback"
	"github.com/GURSEWAK13/claude-connector/internal/gossip"
	"github.com/GURSEWAK13/claude-connector/internal/peer"
	"github.com/GURSEWAK13/claude-connector/internal/proxy"
	"github.com/GURSEWAK13/claude-connector/internal/session"
	"github.com/GURSEWAK13/claude-connector/internal/tui"
	"github.com/GURSEWAK13/claude-connector/internal/web"
)

var (
	noTUI   bool
	webOnly bool
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the claude-connector proxy",
	RunE:  runStart,
}

func init() {
	startCmd.Flags().BoolVar(&noTUI, "no-tui", false, "disable TUI, run headless")
	startCmd.Flags().BoolVar(&webOnly, "web", false, "skip TUI, use web dashboard only")
}

func runStart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Build components ───────────────────────────────────────────────────
	pool := session.NewPool(cfg)
	fb := fallback.NewManager(cfg)
	reg := peer.NewRegistry()
	gossipEngine := gossip.NewEngine(cfg, reg, pool)

	discovery, err := peer.NewDiscovery(cfg, reg)
	if err != nil {
		return fmt.Errorf("peer discovery: %w", err)
	}

	peerServer := peer.NewServer(cfg, pool)
	router := proxy.NewRouter(pool, reg, fb)
	proxyServer := proxy.NewServer(cfg, router)
	webServer := web.NewServer(cfg, pool, reg)
	webServer.SetFallback(fb)

	// ── Start services ─────────────────────────────────────────────────────
	type svcResult struct {
		name string
		err  error
	}
	svcErrCh := make(chan svcResult, 16)

	startSvc := func(name string, fn func(context.Context) error) {
		go func() {
			if err := fn(ctx); err != nil {
				svcErrCh <- svcResult{name, err}
			} else {
				svcErrCh <- svcResult{name, nil}
			}
		}()
	}

	startSvc("pool", pool.Start)
	startSvc("discovery", discovery.Start)
	startSvc("peer-server", peerServer.Start)
	startSvc("gossip", gossipEngine.Start)
	startSvc("proxy", proxyServer.Start)
	startSvc("web", webServer.Start)
	startSvc("fallback", fb.Start)

	// ── Signal handling ────────────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Keep a channel to surface the first fatal service error.
	fatalErrCh := make(chan error, 1)

	// Background watcher: triggers cancel on signal or service crash.
	// Does NOT call os.Exit — just cancels ctx so TUI/headless exits cleanly.
	go func() {
		for {
			select {
			case sig := <-sigCh:
				_ = sig
				cancel()
				return

			case r := <-svcErrCh:
				if r.err != nil {
					fmt.Fprintf(os.Stderr, "\n[%s] fatal: %v\n", r.name, r.err)
					select {
					case fatalErrCh <- r.err:
					default:
					}
					cancel()
					return
				}
				// nil means the service exited cleanly (ctx cancelled) — ignore.

			case <-ctx.Done():
				return
			}
		}
	}()

	// ── Run UI ─────────────────────────────────────────────────────────────
	// IMPORTANT: The TUI runs BLOCKING in this goroutine.
	// This guarantees Bubble Tea always restores the terminal before we
	// return from runStart (and before any os.Exit can fire).
	if !noTUI && !webOnly {
		app := tui.NewApp(cfg, pool, reg, fb)
		uiErr := app.Run(ctx)
		// Only surface unexpected errors, not clean exits (context cancel).
		if uiErr != nil && ctx.Err() == nil {
			return uiErr
		}
	} else {
		fmt.Printf("claude-connector is running\n")
		fmt.Printf("  Proxy   → http://localhost:%d\n", cfg.Proxy.Port)
		fmt.Printf("  Web UI  → http://localhost:%d\n", cfg.Web.Port)
		fmt.Printf("  Peer    → :%d\n", cfg.Peer.Port)
		fmt.Printf("\nPress Ctrl+C to stop.\n")

		select {
		case <-sigCh:
			fmt.Println("\nShutting down...")
		case err := <-fatalErrCh:
			return err
		case <-ctx.Done():
		}
	}

	cancel()

	// Return any fatal service error that was captured.
	select {
	case err := <-fatalErrCh:
		return err
	default:
		return nil
	}
}
