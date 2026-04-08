// Package tui implements the interactive terminal user interface for kin.
package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/niklod/kin/internal/config"
	"github.com/niklod/kin/internal/daemon"
	"github.com/niklod/kin/internal/ipc"
)

var (
	errDaemonDisconnected = errors.New("daemon disconnected")
	errEmptyToken         = errors.New("token is required")
)

// Config holds the parameters needed to launch the TUI.
type Config struct {
	ConfigDir  string
	SharedDir  string
	ListenAddr string
	RelayAddr  string
	Debug      bool
}

// Run starts the interactive TUI. If a daemon is already running, it connects
// to it via IPC. Otherwise, it starts an embedded daemon in-process.
func Run(cfg Config) error {
	if cfg.Debug {
		l, f, err := config.SetupDebugLog(cfg.ConfigDir, false)
		if err != nil {
			return fmt.Errorf("debug log: %w", err)
		}
		defer f.Close()
		slog.SetDefault(l)
	}

	// Try connecting to an existing daemon.
	rpcClient, err := ipc.TryDaemon(cfg.ConfigDir)
	if err == nil {
		defer rpcClient.Close()
		return runTUI(cfg.ConfigDir, rpcClient)
	}

	// No daemon running — start embedded.
	return runEmbedded(cfg)
}

// runTUI connects to an existing daemon and launches bubbletea.
func runTUI(cfgDir string, rpcClient *ipc.Client) error {
	evtClient, err := ipc.TryDaemon(cfgDir)
	if err != nil {
		return fmt.Errorf("event connection failed: %w", err)
	}
	defer evtClient.Close()

	eventCh, err := evtClient.Subscribe()
	if err != nil {
		return fmt.Errorf("subscribe to events: %w", err)
	}

	model := NewModel(rpcClient, eventCh)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}

// runEmbedded starts the daemon in-process, then launches the TUI on top.
func runEmbedded(cfg Config) error {
	sharedDir := cfg.SharedDir
	if sharedDir == "" {
		var err error
		sharedDir, err = config.DefaultSharedDir()
		if err != nil {
			return fmt.Errorf("shared dir: %w", err)
		}
	}

	listenAddr := cfg.ListenAddr
	if listenAddr == "" {
		listenAddr = config.DefaultListenAddr
	}

	// In debug mode, Run() already set up the file logger as slog.Default().
	// Otherwise, suppress verbose output — only warnings reach stderr.
	logger := slog.Default()
	if !cfg.Debug {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}))
	}

	d := daemon.New(cfg.ConfigDir, sharedDir, listenAddr, cfg.RelayAddr, logger)
	d.Embedded = true
	d.ReadyCh = make(chan struct{})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start daemon in background.
	daemonErr := make(chan error, 1)
	go func() {
		daemonErr <- d.Run(ctx)
	}()

	// Wait for daemon to be ready or fail.
	select {
	case <-d.ReadyCh:
		// Daemon initialized, proceed with TUI.
	case err := <-daemonErr:
		return fmt.Errorf("daemon: %w", err)
	}

	// Connect TUI via IPC (same path as external daemon).
	rpcClient, err := ipc.TryDaemon(cfg.ConfigDir)
	if err != nil {
		cancel()
		return fmt.Errorf("connect to embedded daemon: %w", err)
	}
	defer rpcClient.Close()

	tuiErr := runTUI(cfg.ConfigDir, rpcClient)

	// TUI exited ��� shut down the embedded daemon.
	cancel()
	dErr := <-daemonErr

	// Surface daemon error if TUI exited cleanly.
	if tuiErr != nil {
		return tuiErr
	}
	if dErr != nil {
		return fmt.Errorf("daemon: %w", dErr)
	}
	return nil
}
