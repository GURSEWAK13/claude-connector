package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	logTimeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	logLocalStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff87"))
	logPeerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#87afff"))
	logFallbackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaf5f"))
	logErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
	logHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#87afff")).Underline(true)
	logAmberStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaf00"))
)

// LogEntry is a single line in the request log.
type LogEntry struct {
	Time          time.Time
	Via           string
	Status        int
	Duration      string
	Model         string
	Error         string
	FailoverCount int
}

// RenderRequestLog renders the REQUEST LOG panel.
func RenderRequestLog(entries []LogEntry, width, maxLines int) string {
	var sb strings.Builder
	sb.WriteString(logHeaderStyle.Render("REQUEST LOG") + "\n")

	// Show the most recent entries, up to maxLines
	start := 0
	if len(entries) > maxLines {
		start = len(entries) - maxLines
	}

	for _, e := range entries[start:] {
		sb.WriteString(renderLogEntry(e, width))
	}

	return sb.String()
}

func renderLogEntry(e LogEntry, width int) string {
	ts := logTimeStyle.Render(e.Time.Format("15:04:05"))

	var viaStyle lipgloss.Style
	via := e.Via
	switch {
	case via == "local":
		viaStyle = logLocalStyle
	case strings.HasPrefix(via, "peer:"):
		viaStyle = logPeerStyle
	case via == "ollama" || via == "lmstudio":
		viaStyle = logFallbackStyle
	default:
		viaStyle = logErrorStyle
	}

	viaLabel := fmt.Sprintf("→ %-18s", truncate(via, 18))
	viaStr := viaStyle.Render(viaLabel)
	if e.FailoverCount > 0 {
		viaStr += " " + logAmberStyle.Render(fmt.Sprintf("(+%d retry)", e.FailoverCount))
	}

	status := "200"
	statusStyle := logLocalStyle
	if e.Status == 429 {
		status = "429"
		statusStyle = logErrorStyle
	} else if e.Status >= 400 {
		status = fmt.Sprintf("%d", e.Status)
		statusStyle = logErrorStyle
	}

	statusStr := statusStyle.Render(status)
	dur := logTimeStyle.Render(fmt.Sprintf("%-6s", e.Duration))
	model := logTimeStyle.Render(truncate(e.Model, 15))

	return fmt.Sprintf("  %s  %s  %s  %s  %s\n", ts, viaStr, statusStr, dur, model)
}
