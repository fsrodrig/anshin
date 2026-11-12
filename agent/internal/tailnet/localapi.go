package tailnet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultSocket = "/run/tailscale/tailscaled.sock"

// tailscaled checks the Host header against this name to defeat DNS rebinding.
const localAPIHost = "local-tailscaled.sock"

// SocketPath is $TAILSCALE_SOCKET, else tailscaled's default socket.
func SocketPath() string {
	if p := os.Getenv("TAILSCALE_SOCKET"); p != "" {
		return p
	}
	return defaultSocket
}

// LocalAPI is a minimal client of tailscaled's LocalAPI. The official client pulls in about 18 tailscale.com
// packages for the two read-only calls the Agent needs.
type LocalAPI struct {
	http *http.Client
}

func NewLocalAPI(socket string) *LocalAPI {
	var d net.Dialer
	return &LocalAPI{http: &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return d.DialContext(ctx, "unix", socket)
			},
			DisableCompression: true,
		},
	}}
}

func (c *LocalAPI) Self(ctx context.Context) (Self, error) {
	var st struct {
		BackendState string
		Self         *struct {
			UserID       int64
			Tags         []string
			TailscaleIPs []netip.Addr
		}
	}
	if err := c.get(ctx, "/localapi/v0/status?peers=false", &st); err != nil {
		return Self{}, err
	}
	if st.BackendState != "Running" {
		return Self{}, fmt.Errorf("tailscale is not running (backend state %q)", st.BackendState)
	}
	if st.Self == nil || len(st.Self.TailscaleIPs) == 0 {
		return Self{}, fmt.Errorf("tailscaled reported no tailnet addresses for this Machine")
	}
	return Self{
		Node:  Node{UserID: st.Self.UserID, Tags: st.Self.Tags},
		Addrs: st.Self.TailscaleIPs,
	}, nil
}

func (c *LocalAPI) WhoIs(ctx context.Context, addr netip.Addr) (Node, error) {
	var who struct {
		Node *struct {
			User int64
			Tags []string
		}
	}
	err := c.get(ctx, "/localapi/v0/whois?addr="+url.QueryEscape(addr.String()), &who)
	if err != nil {
		return Node{}, err
	}
	if who.Node == nil {
		return Node{}, fmt.Errorf("tailscaled whois for %s returned no node", addr)
	}
	return Node{UserID: who.Node.User, Tags: who.Node.Tags}, nil
}

func (c *LocalAPI) get(ctx context.Context, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+localAPIHost+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tailscaled LocalAPI: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("tailscaled LocalAPI %s: %w", path, err)
	}
	if resp.StatusCode == http.StatusNotFound && strings.HasPrefix(string(body), "no match") {
		return ErrNotPeer
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tailscaled LocalAPI %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("tailscaled LocalAPI %s: %w", path, err)
	}
	return nil
}
