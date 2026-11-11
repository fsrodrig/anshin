package tailnet

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"slices"
	"testing"
)

// stubDaemon serves canned LocalAPI responses on a unix socket, shaped like tailscaled 1.104's.
func stubDaemon(t *testing.T, h http.HandlerFunc) *LocalAPI {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "tailscaled.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return NewLocalAPI(sock)
}

func TestSelf(t *testing.T) {
	c := stubDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host != localAPIHost || r.URL.Path != "/localapi/v0/status" || r.URL.Query().Get("peers") != "false" {
			http.Error(w, "unexpected request "+r.Host+r.URL.String(), http.StatusTeapot)
			return
		}
		w.Write([]byte(`{"BackendState":"Running","Self":{"ID":"n1","UserID":4242,"HostName":"machine-a",
			"TailscaleIPs":["100.64.0.1","fd7a:115c:a1e0::1"]},"User":{"4242":{"LoginName":"owner@example.com"}}}`))
	})
	self, err := c.Self(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Addr{netip.MustParseAddr("100.64.0.1"), netip.MustParseAddr("fd7a:115c:a1e0::1")}
	if self.UserID != 4242 || self.Tagged() || !slices.Equal(self.Addrs, want) {
		t.Fatalf("Self = %+v", self)
	}
}

func TestSelfTagged(t *testing.T) {
	c := stubDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"BackendState":"Running","Self":{"UserID":4242,"Tags":["tag:server"],
			"TailscaleIPs":["100.64.0.1"]}}`))
	})
	self, err := c.Self(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !self.Tagged() {
		t.Fatalf("Self = %+v, want tagged", self)
	}
}

func TestSelfFailsWhenNotRunning(t *testing.T) {
	for name, body := range map[string]string{
		"stopped":      `{"BackendState":"Stopped","Self":{"UserID":4242,"TailscaleIPs":[]}}`,
		"no addresses": `{"BackendState":"Running","Self":{"UserID":4242,"TailscaleIPs":[]}}`,
		"no self":      `{"BackendState":"Running"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := stubDaemon(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(body)) })
			if _, err := c.Self(context.Background()); err == nil {
				t.Fatal("Self succeeded, want an error")
			}
		})
	}
}

func TestSelfFailsWithoutDaemon(t *testing.T) {
	c := NewLocalAPI(filepath.Join(t.TempDir(), "missing.sock"))
	if _, err := c.Self(context.Background()); err == nil {
		t.Fatal("Self succeeded, want an error")
	}
}

func TestWhoIs(t *testing.T) {
	c := stubDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("addr") {
		case "100.64.0.2":
			w.Write([]byte(`{"Node":{"ID":7,"User":4242,"Name":"phone.example.ts.net."},
				"UserProfile":{"ID":4242,"LoginName":"owner@example.com"},"CapMap":{}}`))
		case "fd7a:115c:a1e0::3":
			w.Write([]byte(`{"Node":{"ID":8,"User":4242,"Tags":["tag:ci"]},"UserProfile":{"ID":4242}}`))
		default:
			http.Error(w, "no match for IP:port", http.StatusNotFound)
		}
	})
	ctx := context.Background()

	n, err := c.WhoIs(ctx, netip.MustParseAddr("100.64.0.2"))
	if err != nil || n.UserID != 4242 || n.Tagged() {
		t.Fatalf("WhoIs(phone) = %+v, %v", n, err)
	}
	n, err = c.WhoIs(ctx, netip.MustParseAddr("fd7a:115c:a1e0::3"))
	if err != nil || !n.Tagged() {
		t.Fatalf("WhoIs(tagged) = %+v, %v", n, err)
	}
	if _, err := c.WhoIs(ctx, netip.MustParseAddr("100.64.0.9")); !errors.Is(err, ErrNotPeer) {
		t.Fatalf("WhoIs(stranger) error = %v, want ErrNotPeer", err)
	}
}

func TestWhoIsDaemonErrorIsNotNotPeer(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"500": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		},
		"404 without no match": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "404 page not found", http.StatusNotFound)
		},
		"garbage": func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("<html>")) },
	} {
		t.Run(name, func(t *testing.T) {
			c := stubDaemon(t, h)
			_, err := c.WhoIs(context.Background(), netip.MustParseAddr("100.64.0.2"))
			if err == nil || errors.Is(err, ErrNotPeer) {
				t.Fatalf("WhoIs error = %v, want a daemon error", err)
			}
		})
	}
}
