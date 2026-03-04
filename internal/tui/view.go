package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/GURSEWAK13/claude-connector/internal/fallback"
	"github.com/GURSEWAK13/claude-connector/internal/tui/components"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#87d7ff"))
	borderStyle = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#444444"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#444444"))
)

// View renders the full TUI.
func (m *Model) View() string {
	if m.quitting {
		return "Shutting down claude-connector...\n"
	}

	if m.width < 60 || m.height < 15 {
		return m.renderCompact()
	}

	return m.renderFull()
}

func (m *Model) renderFull() string {
	// Header
	uptime := time.Since(m.startTime).Round(time.Second)
	header := titleStyle.Render(fmt.Sprintf(
		" claude-connector v1.0  node:%s  proxy::%d  uptime:%v ",
		m.cfg.NodeName, m.cfg.Proxy.Port, uptime,
	))

	// Left panel: Sessions + Fallback
	leftWidth := m.width/3 - 2
	leftContent := components.RenderSessions(m.sessions, leftWidth)
	leftContent += components.RenderFallback(m.fallbackStatuses(), leftWidth)

	// Right panel: Peer Network (top) + Request Log (bottom)
	rightWidth := m.width - leftWidth - 5
	graphHeight := m.height/2 - 6
	if graphHeight < 5 {
		graphHeight = 5
	}

	networkTitle := dimStyle.Render("PEER NETWORK")
	graphContent := networkTitle + "\n" + components.ColoredGraph(
		m.cfg.NodeName, m.peers, rightWidth, graphHeight,
	)

	logHeight := m.height - graphHeight - 10
	if logHeight < 3 {
		logHeight = 3
	}
	logContent := components.RenderRequestLog(m.requestLog, rightWidth, logHeight)

	// Layout using lipgloss Join
	leftPanel := lipgloss.NewStyle().
		Width(leftWidth).
		Height(m.height - 6).
		Render(leftContent)

	rightPanel := lipgloss.NewStyle().
		Width(rightWidth).
		Height(m.height - 6).
		Render(graphContent + "\n" + strings.Repeat("─", rightWidth) + "\n" + logContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		leftPanel,
		dimStyle.Render(" │ "),
		rightPanel,
	)

	// Footer
	footer := helpStyle.Render(" [q]uit  [w]eb-dashboard  [?]help")

	// Border the whole thing
	inner := header + "\n" + strings.Repeat("─", m.width-2) + "\n" + body + "\n" + strings.Repeat("─", m.width-2) + "\n" + footer
	return borderStyle.Width(m.width - 2).Render(inner)
}

func (m *Model) renderCompact() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("claude-connector") + "\n")
	sb.WriteString(components.RenderSessions(m.sessions, m.width) + "\n")
	sb.WriteString(components.RenderPeerList(m.peers, m.width) + "\n")
	return sb.String()
}

func (m *Model) fallbackStatuses() []components.FallbackStatus {
	statuses := m.fbManager.Statuses()
	result := make([]components.FallbackStatus, len(statuses))
	for i, s := range statuses {
		result[i] = components.FallbackStatus{
			Name:      s.Name,
			Available: s.Available,
		}
	}
	return result
}

// convertFallbackStatus is a typed helper that ensures the fallback import is used.
func convertFallbackStatus(s fallback.BackendStatus) components.FallbackStatus {
	return components.FallbackStatus{
		Name:      s.Name,
		Available: s.Available,
	}
}
