package tailnet

import (
	"context"
	"net/netip"
	"sync"
)

// Fake is an in-memory tailnet. Addresses not in Peers are not tailnet peers.
type Fake struct {
	mu       sync.Mutex
	self     Self
	selfErr  error
	peers    map[netip.Addr]Node
	whoisErr error
	whois    int
}

func NewFake(self Self) *Fake {
	return &Fake{self: self, peers: map[netip.Addr]Node{}}
}

func (f *Fake) SetSelf(self Self, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.self, f.selfErr = self, err
}

func (f *Fake) SetPeer(addr netip.Addr, n Node) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peers[addr] = n
}

// FailWhoIs makes every WhoIs fail with err, as when tailscaled is down; nil restores it.
func (f *Fake) FailWhoIs(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.whoisErr = err
}

// WhoIsCalls counts the WhoIs calls that reached the fake.
func (f *Fake) WhoIsCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.whois
}

func (f *Fake) Self(ctx context.Context) (Self, error) {
	if err := ctx.Err(); err != nil {
		return Self{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.self, f.selfErr
}

func (f *Fake) WhoIs(_ context.Context, addr netip.Addr) (Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.whois++
	if f.whoisErr != nil {
		return Node{}, f.whoisErr
	}
	n, ok := f.peers[addr]
	if !ok {
		return Node{}, ErrNotPeer
	}
	return n, nil
}
