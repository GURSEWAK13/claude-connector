package peer

import (
	"context"
	"fmt"
	"net"

	"github.com/grandcat/zeroconf"
	"github.com/GURSEWAK13/claude-connector/internal/config"
)

const mdnsService = "_claude-connector._tcp"
const mdnsDomain = "local."

// Discovery handles mDNS-based peer discovery.
type Discovery struct {
	cfg      *config.Config
	registry *Registry
	server   *zeroconf.Server
}

func NewDiscovery(cfg *config.Config, reg *Registry) (*Discovery, error) {
	return &Discovery{cfg: cfg, registry: reg}, nil
}

// Start advertises this node via mDNS and discovers peers.
func (d *Discovery) Start(ctx context.Context) error {
	// Register our service
	server, err := zeroconf.Register(
		d.cfg.NodeName,
		mdnsService,
		mdnsDomain,
		d.cfg.Peer.Port,
		[]string{
			"id=" + d.cfg.NodeID,
			"name=" + d.cfg.NodeName,
			"available=0",
		},
		nil,
	)
	if err != nil {
		return fmt.Errorf("mDNS register: %w", err)
	}
	d.server = server

	// Browse for peers
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("mDNS resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry, 16)
	go func() {
		for entry := range entries {
			d.handleEntry(entry)
		}
	}()

	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()

	if err := resolver.Browse(ctx, mdnsService, mdnsDomain, entries); err != nil {
		return fmt.Errorf("mDNS browse: %w", err)
	}

	// Browse is async — block until context is cancelled.
	<-ctx.Done()
	return nil
}

func (d *Discovery) handleEntry(entry *zeroconf.ServiceEntry) {
	// Skip ourselves
	if entry.Instance == d.cfg.NodeName {
		return
	}

	// Parse TXT records
	info := PeerInfo{
		Name:      entry.Instance,
		Port:      entry.Port,
		Available: true,
	}

	for _, txt := range entry.Text {
		switch {
		case len(txt) > 3 && txt[:3] == "id=":
			info.ID = txt[3:]
		case len(txt) > 10 && txt[:10] == "available=":
			n := 0
			fmt.Sscanf(txt[10:], "%d", &n)
			info.AvailableSessions = n
			info.Available = n > 0
		}
	}

	if info.ID == "" {
		info.ID = entry.Instance
	}

	// Resolve host address
	if len(entry.AddrIPv4) > 0 {
		info.Host = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		info.Host = "[" + entry.AddrIPv6[0].String() + "]"
	} else {
		// Try to resolve from hostname
		addrs, err := net.LookupHost(entry.HostName)
		if err != nil || len(addrs) == 0 {
			return
		}
		info.Host = addrs[0]
	}

	d.registry.Upsert(info)
}
