package components

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/GURSEWAK13/claude-connector/internal/peer"
)

var (
	graphNodeAvail = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff87")).Bold(true)
	graphNodeBusy  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffff87")).Bold(true)
	graphNodeDead  = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	graphNodeSelf  = lipgloss.NewStyle().Foreground(lipgloss.Color("#87d7ff")).Bold(true)
	graphEdgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#444444"))
)

// Node represents a node in the ASCII graph.
type Node struct {
	Name      string
	X, Y      float64
	Available bool
	Sessions  int
	IsSelf    bool
}

// RenderNetworkGraph renders an ASCII force-layout peer graph within the given dimensions.
// It places nodes in a circular layout around the self node.
func RenderNetworkGraph(selfName string, peers []peer.PeerInfo, width, height int) string {
	if width < 20 || height < 8 {
		return RenderPeerList(peers, width)
	}

	nodes := buildNodes(selfName, peers)
	if len(nodes) == 0 {
		return graphNodeSelf.Render("  ["+selfName+"] ") + "\n" + graphNodeDead.Render("  No peers\n")
	}

	// Simple circular layout: self in center, peers around it
	grid := newGrid(width, height)
	cx := float64(width) / 2
	cy := float64(height) / 2

	// Place self in center
	selfNode := &nodes[0]
	selfNode.X = cx
	selfNode.Y = cy

	// Place peers in a circle around self
	peerCount := len(nodes) - 1
	if peerCount > 0 {
		radius := math.Min(cx-4, cy-2)
		for i := 1; i <= peerCount; i++ {
			angle := (2 * math.Pi * float64(i-1)) / float64(peerCount)
			nodes[i].X = cx + radius*math.Cos(angle)
			nodes[i].Y = cy + radius*math.Sin(angle)*0.5 // flatten vertically
		}
	}

	// Draw edges from self to each peer
	for i := 1; i < len(nodes); i++ {
		drawEdge(grid, nodes[0], nodes[i], width, height)
	}

	// Draw node labels
	for _, n := range nodes {
		drawNode(grid, n, width, height)
	}

	// Render grid to string
	var sb strings.Builder
	for y := 0; y < height; y++ {
		sb.WriteString(string(grid[y]) + "\n")
	}
	return sb.String()
}

type grid [][]rune

func newGrid(w, h int) grid {
	g := make(grid, h)
	for i := range g {
		g[i] = make([]rune, w)
		for j := range g[i] {
			g[i][j] = ' '
		}
	}
	return g
}

func drawEdge(g grid, a, b Node, w, h int) {
	x1, y1 := int(a.X), int(a.Y)
	x2, y2 := int(b.X), int(b.Y)

	// Bresenham line
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	sx, sy := 1, 1
	if x1 > x2 {
		sx = -1
	}
	if y1 > y2 {
		sy = -1
	}

	err := dx - dy
	for {
		if x1 == x2 && y1 == y2 {
			break
		}
		if x1 >= 0 && x1 < w && y1 >= 0 && y1 < h {
			if g[y1][x1] == ' ' {
				if dx > dy {
					g[y1][x1] = '─'
				} else {
					g[y1][x1] = '│'
				}
			}
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

func drawNode(g grid, n Node, w, h int) {
	x, y := int(n.X), int(n.Y)
	label := fmt.Sprintf("[%s]", truncate(n.Name, 8))
	sessions := fmt.Sprintf("%dav", n.Sessions)

	for i, ch := range label {
		px := x - len(label)/2 + i
		if px >= 0 && px < w && y >= 0 && y < h {
			g[y][px] = ch
		}
	}
	// Sessions on line below
	for i, ch := range sessions {
		px := x - len(sessions)/2 + i
		if px >= 0 && px < w && y+1 >= 0 && y+1 < h {
			g[y+1][px] = ch
		}
	}
}

func buildNodes(selfName string, peers []peer.PeerInfo) []Node {
	nodes := []Node{
		{Name: selfName, Available: true, Sessions: 1, IsSelf: true},
	}
	for _, p := range peers {
		nodes = append(nodes, Node{
			Name:      p.Name,
			Available: p.Available,
			Sessions:  p.AvailableSessions,
		})
	}
	return nodes
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ColoredGraph renders a styled version of the graph using lipgloss.
// For simplicity in the terminal, this just renders the ASCII grid with color applied to node labels.
func ColoredGraph(selfName string, peers []peer.PeerInfo, width, height int) string {
	raw := RenderNetworkGraph(selfName, peers, width, height)
	// Return raw (colors would require cell-by-cell rendering; ASCII is sufficient)
	_ = graphNodeAvail
	_ = graphNodeBusy
	_ = graphNodeDead
	_ = graphNodeSelf
	_ = graphEdgeStyle
	return raw
}
