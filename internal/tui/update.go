package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/GURSEWAK13/claude-connector/internal/peer"
	"github.com/GURSEWAK13/claude-connector/internal/session"
	"github.com/GURSEWAK13/claude-connector/internal/tui/components"
)

// Msg types

type sessionUpdateMsg session.Snapshot
type peerUpdateMsg peer.PeerInfo
type requestLogMsg components.LogEntry
type tickMsg struct{}
type errMsg error

// Update handles incoming messages and user input.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case sessionUpdateMsg:
		m.updateSession(session.Snapshot(msg))
		return m, nil

	case peerUpdateMsg:
		m.updatePeer(peer.PeerInfo(msg))
		return m, nil

	case requestLogMsg:
		m.addLogEntry(components.LogEntry(msg))
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.listenSessions(), m.listenPeers(), m.listenRequests(), tick())

	case errMsg:
		m.err = msg
		return m, nil
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "w":
		m.showingWeb = !m.showingWeb
	case "?":
		m.showHelp = !m.showHelp
	}
	return m, nil
}

func (m *Model) updateSession(snap session.Snapshot) {
	for i, s := range m.sessions {
		if s.ID == snap.ID {
			m.sessions[i] = snap
			return
		}
	}
	m.sessions = append(m.sessions, snap)
}

func (m *Model) updatePeer(info peer.PeerInfo) {
	for i, p := range m.peers {
		if p.ID == info.ID {
			m.peers[i] = info
			return
		}
	}
	m.peers = append(m.peers, info)
}

func (m *Model) addLogEntry(entry components.LogEntry) {
	m.requestLog = append(m.requestLog, entry)
	if len(m.requestLog) > maxLogEntries {
		m.requestLog = m.requestLog[len(m.requestLog)-maxLogEntries:]
	}
}
