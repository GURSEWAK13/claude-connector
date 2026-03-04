package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/GURSEWAK13/claude-connector/internal/session"
)

var (
	sessionAvailStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff87")).Bold(true)
	sessionInUseStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffff87"))
	sessionCoolStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff875f"))
	sessionRLStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
	sessionDisabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	sessionLabelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d7d7d7"))
	sectionTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#87afff")).Underline(true)
)

// RenderSessions renders the SESSIONS panel.
func RenderSessions(snapshots []session.Snapshot, width int) string {
	var sb strings.Builder
	sb.WriteString(sectionTitleStyle.Render("SESSIONS") + "\n")

	if len(snapshots) == 0 {
		sb.WriteString(sessionDisabledStyle.Render("  No sessions configured") + "\n")
		return sb.String()
	}

	for _, s := range snapshots {
		sb.WriteString(renderSession(s, width))
	}
	return sb.String()
}

func renderSession(s session.Snapshot, width int) string {
	bullet := "●"
	label := sessionLabelStyle.Render(fmt.Sprintf("  %-20s", truncate(s.ID, 20)))

	var stateStr string
	switch s.State {
	case session.StateAvailable:
		stateStr = sessionAvailStyle.Render(bullet + " AVAILABLE")
	case session.StateInUse:
		stateStr = sessionInUseStyle.Render(bullet + " IN USE")
	case session.StateRateLimited:
		stateStr = sessionRLStyle.Render(bullet + " RATE LIMITED")
	case session.StateCoolingDown:
		remaining := s.CooldownRemaining.Round(time.Second)
		stateStr = sessionCoolStyle.Render(fmt.Sprintf(bullet+" COOLING %v", remaining))
	}

	if !s.Enabled {
		stateStr = sessionDisabledStyle.Render("○ DISABLED")
	}

	return label + stateStr + "\n"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// RenderFallback renders the FALLBACK panel.
func RenderFallback(statuses []FallbackStatus, width int) string {
	var sb strings.Builder
	sb.WriteString("\n" + sectionTitleStyle.Render("FALLBACK") + "\n")

	for _, s := range statuses {
		bullet := sessionAvailStyle.Render("●")
		check := sessionAvailStyle.Render("✓")
		if !s.Available {
			bullet = sessionDisabledStyle.Render("○")
			check = sessionRLStyle.Render("✗")
		}
		sb.WriteString(fmt.Sprintf("  %s %-12s %s\n",
			bullet,
			sessionLabelStyle.Render(s.Name),
			check,
		))
	}
	return sb.String()
}

// FallbackStatus holds display state for a fallback backend.
type FallbackStatus struct {
	Name      string
	Available bool
	URL       string
}
