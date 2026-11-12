// Package tailnet answers who this Machine is on the tailnet and who a caller is. It is the Agent's only source of
// identity: there are no tokens (ADR 0002).
package tailnet

import (
	"context"
	"errors"
	"net/netip"
)

// Client is the seam between the Agent and tailscaled. LocalAPI talks to the real daemon; Fake stands in for it in
// tests.
type Client interface {
	Self(ctx context.Context) (Self, error)
	// WhoIs returns ErrNotPeer when addr belongs to no tailnet node. Any other error means tailscaled could not
	// answer.
	WhoIs(ctx context.Context, addr netip.Addr) (Node, error)
}

// ErrNotPeer reports that an address belongs to no node on this tailnet.
var ErrNotPeer = errors.New("not a tailnet peer")

// Node is a tailnet node's identity as far as authorization cares.
type Node struct {
	// UserID is the user who created the node. For a tagged node that says nothing about ownership.
	UserID int64
	Tags   []string
}

func (n Node) Tagged() bool { return len(n.Tags) > 0 }

// Self is this Machine's own node and its tailnet addresses.
type Self struct {
	Node
	Addrs []netip.Addr
}
