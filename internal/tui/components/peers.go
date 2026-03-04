package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/GURSEWAK13/claude-connector/internal/peer"
)

var (
	peerAvailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff87"))
	peerBusyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffff87"))
	peerDeadStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	peerNameStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#afffff"))
)

// RenderPeerList renders a simple text list of peers (fallback when graph is too small).
func RenderPeerList(peers []peer.PeerInfo, width int) string {
	var sb strings.Builder
	for _, p := range peers {
		var style lipgloss.Style
		var marker string
		if p.Available && p.AvailableSessions > 0 {
			style = peerAvailStyle
			marker = "●"
		} else if p.AvailableSessions == 0 {
			style = peerDeadStyle
			marker = "○"
		} else {
			style = peerBusyStyle
			marker = "◐"
		}
		sb.WriteString(fmt.Sprintf("  %s %s  %d avail\n",
			style.Render(marker),
			peerNameStyle.Render(truncate(p.Name, 16)),
			p.AvailableSessions,
		))
	}
	if sb.Len() == 0 {
		sb.WriteString(peerDeadStyle.Render("  No peers discovered\n"))
	}
	return sb.String()
}
