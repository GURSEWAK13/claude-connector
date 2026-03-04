package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/GURSEWAK13/claude-connector/internal/config"
	"github.com/GURSEWAK13/claude-connector/internal/fallback"
	"github.com/GURSEWAK13/claude-connector/internal/peer"
	"github.com/GURSEWAK13/claude-connector/internal/session"
	"github.com/GURSEWAK13/claude-connector/internal/tui/components"
)

const maxLogEntries = 100

// Model is the root Bubble Tea model.
type Model struct {
	cfg       *config.Config
	pool      *session.Pool
	registry  *peer.Registry
	fbManager *fallback.Manager

	sessions   []session.Snapshot
	peers      []peer.PeerInfo
	requestLog []components.LogEntry

	// Subscription channels
	sessionCh <-chan session.Snapshot
	peerCh    <-chan peer.PeerInfo

	// UI state
	width      int
	height     int
	startTime  time.Time
	quitting   bool
	showingWeb bool
	showHelp   bool
	err        error
}

// App wraps the Bubble Tea program.
type App struct {
	model *Model
	prog  *tea.Program
}

func NewApp(cfg *config.Config, pool *session.Pool, registry *peer.Registry, fb *fallback.Manager) *App {
	m := &Model{
		cfg:       cfg,
		pool:      pool,
		registry:  registry,
		fbManager: fb,
		startTime: time.Now(),
		sessionCh: pool.Subscribe(),
		peerCh:    registry.Subscribe(),
	}

	// Load initial state
	m.sessions = pool.Snapshots()
	m.peers = registry.All()

	prog := tea.NewProgram(m, tea.WithAltScreen())
	return &App{model: m, prog: prog}
}

func (a *App) Run(ctx context.Context) error {
	// Watch for context cancellation
	go func() {
		<-ctx.Done()
		a.prog.Quit()
	}()

	_, err := a.prog.Run()
	return err
}

// Init is called by Bubble Tea to get the initial command.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.listenSessions(),
		m.listenPeers(),
		m.listenRequests(),
		tick(),
	)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *Model) listenSessions() tea.Cmd {
	return func() tea.Msg {
		select {
		case snap, ok := <-m.sessionCh:
			if !ok {
				return nil
			}
			return sessionUpdateMsg(snap)
		default:
			return nil
		}
	}
}

func (m *Model) listenPeers() tea.Cmd {
	return func() tea.Msg {
		select {
		case info, ok := <-m.peerCh:
			if !ok {
				return nil
			}
			return peerUpdateMsg(info)
		default:
			return nil
		}
	}
}

func (m *Model) listenRequests() tea.Cmd {
	// Request log is populated via AddRequestEvent; no channel needed at init
	return nil
}

// AddRequestEvent adds a request log entry from outside the TUI (e.g. proxy handler).
func (a *App) AddRequestEvent(entry components.LogEntry) {
	a.prog.Send(requestLogMsg(entry))
}
