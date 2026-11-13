package qa

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Resource states. A resource is recorded as pending before the command that creates it runs, so a run that dies
// half way still names everything `down` may have to remove.
const (
	statePending = "pending"
	stateActive  = "active"
	stateRemoved = "removed"
	stateFailed  = "cleanup-failed"
)

// Resource kinds, in the order up creates them; down undoes them in reverse.
const (
	kindRoot         = "adb-root"
	kindMarker       = "device-owner-marker"
	kindSettings     = "device-settings"
	kindInstall      = "app-install"
	kindHostAgent    = "qa-agent-host"
	kindReverse      = "adb-reverse"
	kindDNAT         = "iptables-dnat"
	kindDeviceDir    = "device-dir"
	kindAlias        = "loopback-alias"
	kindDeviceAgent  = "qa-agent-device"
	kindSeed         = "theme-seed"
	ledgerFile       = "ledger.json"
	ledgerSchemaVers = 1
)

type Resource struct {
	ID      int               `json:"id"`
	Kind    string            `json:"kind"`
	State   string            `json:"state"`
	Data    map[string]string `json:"data"`
	Error   string            `json:"error,omitempty"`
	Created time.Time         `json:"created"`
	Updated time.Time         `json:"updated"`
}

// Ledger is the run's record of what it owns. Only what it lists is ever removed.
type Ledger struct {
	SchemaVersion int         `json:"schemaVersion"`
	RunID         string      `json:"runId"`
	Serial        string      `json:"serial,omitempty"`
	Route         string      `json:"route,omitempty"`
	Resources     []*Resource `json:"resources"`
	// Failures records why up stopped, so finish can tell a broken environment from a missing observation.
	Failures []string `json:"failures,omitempty"`

	path string
	now  func() time.Time
}

func loadLedger(run string, runID string) (*Ledger, error) {
	l := &Ledger{SchemaVersion: ledgerSchemaVers, RunID: runID, path: filepath.Join(run, ledgerFile), now: time.Now}
	b, err := os.ReadFile(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, l); err != nil {
		return nil, fmt.Errorf("%s: %w", l.path, err)
	}
	if l.RunID != runID {
		return nil, fmt.Errorf("%s belongs to run %q, not %q", l.path, l.RunID, runID)
	}
	return l, nil
}

func (l *Ledger) save() error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(l.path, append(b, '\n'), 0o600)
}

// begin records a resource as pending and saves the ledger before the caller creates it.
func (l *Ledger) begin(kind string, data map[string]string) (*Resource, error) {
	now := l.now().UTC()
	r := &Resource{ID: len(l.Resources) + 1, Kind: kind, State: statePending, Data: data, Created: now, Updated: now}
	l.Resources = append(l.Resources, r)
	return r, l.save()
}

func (l *Ledger) set(r *Resource, state string, cause error) error {
	r.State, r.Updated = state, l.now().UTC()
	r.Error = ""
	if cause != nil {
		r.Error = cause.Error()
	}
	return l.save()
}

// live returns the resources of kind that may still exist, newest first.
func (l *Ledger) live(kind string) []*Resource {
	var out []*Resource
	for i := len(l.Resources) - 1; i >= 0; i-- {
		r := l.Resources[i]
		if r.Kind == kind && (r.State == statePending || r.State == stateActive || r.State == stateFailed) {
			out = append(out, r)
		}
	}
	return out
}

func (l *Ledger) owns(kind string) bool { return len(l.live(kind)) > 0 }

func (l *Ledger) fail(msg string) error {
	l.Failures = append(l.Failures, msg)
	return l.save()
}

func writeFileAtomic(path string, b []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
