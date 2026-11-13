// Package capture copies the files probe reads on a real Machine into a scrubbed, replayable tree under
// contract/captures, for the probe replay tests and the golden fixtures. It only reads the Machine; it writes
// nothing outside the capture directory and needs no root.
package capture

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/fsrodrig/anshin/agent/internal/recorder/probe"
)

type Source struct {
	Host probe.Host
	// MountID is statx(AT_NO_AUTOMOUNT, STATX_MNT_ID), recorded now so #4's visibility check can be replayed.
	MountID func(path string) (uint64, error)
	Clock   func() Clock
	User    string
}

type Options struct {
	Name                    string
	Snapshots               int
	Interval, StatfsTimeout time.Duration
	Identifiers             []byte
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)

// Run takes opt.Snapshots snapshots of the Machine into dir, which must be missing or empty, and writes a review
// report to report. The report never prints a raw identifier, so it can be pasted into a pull request.
func Run(ctx context.Context, src Source, dir string, opt Options, report io.Writer) error {
	if err := opt.validate(); err != nil {
		return err
	}
	if src.MountID == nil {
		return errors.New("capture requires a mount visibility check")
	}
	if err := ensureEmpty(dir); err != nil {
		return err
	}
	terms, err := gatherTerms(src, opt.Identifiers)
	if err != nil {
		return err
	}
	scr, err := NewScrubber(opt.Name, terms)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(dir), ".capture-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	root, err := os.OpenRoot(staging)
	if err != nil {
		return err
	}
	defer root.Close()

	c := &capturer{src: src, opt: opt, scr: scr, root: root, claimed: map[string]string{}}
	manifest := Manifest{Format: FormatVersion, CPUsOnline: cpusOnline(src.Host)}
	var snaps []*snapshot
	start := time.Now()
	for i := range opt.Snapshots {
		if i > 0 {
			// Scheduled from the start rather than slept after each snapshot, so the time a snapshot takes doesn't
			// stretch the interval the recorder sees between them.
			t := time.NewTimer(time.Until(start.Add(time.Duration(i) * opt.Interval)))
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}
		clock := src.Clock()
		clock.Wall = clock.Wall.UTC()
		manifest.Snapshots = append(manifest.Snapshots, clock)
		if err := ctx.Err(); err != nil {
			return err
		}
		s := c.take()
		h := strings.TrimSpace(string(s.files["proc/sys/kernel/hostname"]))
		if !slices.ContainsFunc(terms.Hostnames, func(known string) bool { return strings.EqualFold(h, known) }) {
			return errors.New("kernel hostname changed or became unreadable during capture")
		}
		if err := c.write(strconv.Itoa(i), s); err != nil {
			return err
		}
		snaps = append(snaps, s)
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := c.writeFile(ManifestFile, append(b, '\n')); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ensureEmpty(dir); err != nil {
		return err
	}
	if _, err := os.Lstat(dir); err == nil {
		if err := os.Remove(dir); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Rename(staging, dir); err != nil {
		return err
	}
	c.report(report, snaps)
	return nil
}

func (o Options) validate() error {
	if !nameRe.MatchString(o.Name) {
		return fmt.Errorf("-name %q must match %s", o.Name, nameRe)
	}
	if slices.Contains(reservedNames, o.Name) {
		return fmt.Errorf("-name %q is reserved in contract/fixtures", o.Name)
	}
	if o.Snapshots < 2 {
		return errors.New("-snapshots must be at least 2: the recorder needs two readings for a CPU %")
	}
	if o.Interval <= 0 || o.StatfsTimeout <= 0 {
		return errors.New("-interval and -statfs-timeout must be positive")
	}
	return nil
}

func ensureEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s is not empty; capture into a new directory", dir)
	}
	return nil
}

// gatherTerms reads the Machine's own names. /etc/hostname and /etc/passwd are read only for this and never
// copied.
func gatherTerms(src Source, identifiers []byte) (Terms, error) {
	var t Terms
	addHost := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" {
			return
		}
		t.Hostnames = append(t.Hostnames, h)
		if short, _, ok := strings.Cut(h, "."); ok && short != "" {
			t.Hostnames = append(t.Hostnames, short)
		}
	}
	h, err := probe.Hostname(src.Host)
	if err != nil || strings.TrimSpace(h) == "" {
		return t, errors.New("cannot learn kernel hostname before capture")
	}
	addHost(h)
	if b, err := fs.ReadFile(src.Host.FS, "etc/hostname"); err == nil {
		addHost(string(b))
	}
	t.Users = append(t.Users, src.User)
	if b, err := fs.ReadFile(src.Host.FS, "etc/passwd"); err == nil {
		for line := range strings.SplitSeq(string(b), "\n") {
			f := strings.Split(line, ":")
			if len(f) < 3 {
				continue
			}
			// Login accounts only: system accounts are below 1000 and nobody is 65534.
			if uid, err := strconv.Atoi(f[2]); err == nil && uid >= 1000 && uid < 65534 {
				t.Users = append(t.Users, f[0])
			}
		}
	}
	for line := range strings.SplitSeq(string(identifiers), "\n") {
		line, _, _ = strings.Cut(line, "#")
		line = strings.Join(strings.Fields(line), "")
		if name, ok := strings.CutPrefix(line, "user:"); ok {
			t.Users = append(t.Users, name)
		} else if line != "" {
			t.Words = append(t.Words, line)
		}
	}
	return t, nil
}

func cpusOnline(h probe.Host) *int {
	b, err := fs.ReadFile(h.FS, "sys/devices/system/cpu/online")
	if err != nil {
		return nil
	}
	n := 0
	for r := range strings.SplitSeq(strings.TrimSpace(string(b)), ",") {
		lo, hi, isRange := strings.Cut(r, "-")
		if !isRange {
			hi = lo
		}
		a, err1 := strconv.Atoi(lo)
		z, err2 := strconv.Atoi(hi)
		if err1 != nil || err2 != nil || z < a {
			return nil
		}
		n += z - a + 1
	}
	return &n
}

type capturer struct {
	src     Source
	opt     Options
	scr     *Scrubber
	root    *os.Root
	pending map[string]chan StatfsRecord
	claimed map[string]string
}

type snapshot struct {
	entries []Entry
	links   map[string]string
	seen    map[string]bool
	missing map[string]error
	files   map[string][]byte
	sizes   map[string]int
	skips   []string
	statfs  map[string]StatfsRecord
}

func (c *capturer) take() *snapshot {
	s := &snapshot{links: map[string]string{}, seen: map[string]bool{}, missing: map[string]error{}, files: map[string][]byte{},
		sizes: map[string]int{}, statfs: map[string]StatfsRecord{}}
	w := walker{fsys: c.src.Host.FS, s: s}
	for _, f := range fixedFiles {
		w.readFile(f)
	}
	for _, d := range w.readDir("sys/class/hwmon") {
		base := "sys/class/hwmon/" + d
		if !w.visit(base) {
			continue
		}
		nameBytes := w.readFile(base + "/name")
		if nameBytes == nil {
			s.skips = append(s.skips, base+": unreadable name")
			continue
		}
		name := strings.TrimSpace(string(nameBytes))
		if storageSensors[name] {
			s.skips = append(s.skips, base+": "+hwmonSkip(name, "", ""))
			continue
		}
		link, err := w.link(base + "/device")
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			s.skips = append(s.skips, base+": unreadable device link")
			continue
		}
		var status string
		if link != "" {
			statusPath := base + "/" + hwmonRuntimeStatus
			b, err := w.readFileErr(statusPath)
			status = strings.TrimSpace(string(b))
			if err != nil && (!errors.Is(err, fs.ErrNotExist) || s.missing[path.Dir(statusPath)] != nil) {
				s.skips = append(s.skips, base+": unreadable runtime status")
				continue
			}
		}
		if why := hwmonSkip(name, link, status); why != "" {
			s.skips = append(s.skips, base+": "+why)
			continue
		}
		for _, attr := range w.readDir(base) {
			if tempAttr.MatchString(attr) {
				w.readFile(base + "/" + attr)
			}
		}
	}
	attrs := slices.Sorted(maps.Keys(supplyAttrs))
	for _, d := range w.readDir("sys/class/power_supply") {
		base := "sys/class/power_supply/" + d
		if !w.visit(base) {
			continue
		}
		for _, a := range attrs {
			w.readFile(base + "/" + a)
		}
	}
	for _, class := range []string{"net", "block"} {
		for _, d := range w.readDir("sys/class/" + class) {
			if base := "sys/class/" + class + "/" + d; w.visit(base) {
				_, _ = w.link(base + "/device")
			}
		}
	}
	c.statfsAll(s)
	return s
}

// walker records every directory and link it passes through, so replay can rebuild the same tree, link types
// included.
type walker struct {
	fsys fs.FS
	s    *snapshot
}

func (w walker) add(e Entry) { w.s.entries = append(w.s.entries, e) }

func (w walker) visit(p string) bool {
	if w.s.missing[p] != nil {
		return false
	}
	if w.s.seen[p] {
		return true
	}
	if dir := path.Dir(p); dir != "." && !w.visit(dir) {
		w.s.missing[p] = w.s.missing[dir]
		return false
	}
	fi, err := fs.Lstat(w.fsys, p)
	if err != nil {
		w.s.missing[p] = err
		if !errors.Is(err, fs.ErrNotExist) {
			w.add(Entry{Kind: Error, Path: p, Arg: errnoName(err)})
		}
		return false
	}
	w.s.seen[p] = true
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		target, err := fs.ReadLink(w.fsys, p)
		if err != nil {
			w.s.missing[p] = err
			w.add(Entry{Kind: Error, Path: p, Arg: errnoName(err)})
			return false
		}
		w.s.links[p] = path.Base(target)
		_, targetErr := fs.Stat(w.fsys, p)
		w.add(Entry{Kind: Link, Path: p, Arg: path.Base(target), Dangling: errors.Is(targetErr, fs.ErrNotExist)})
	case fi.IsDir():
		w.add(Entry{Kind: Dir, Path: p})
	}
	return true
}

func (w walker) link(p string) (string, error) {
	if !w.visit(p) {
		return "", w.s.missing[p]
	}
	return w.s.links[p], nil
}

func (w walker) readFile(p string) []byte {
	b, _ := w.readFileErr(p)
	return b
}

func (w walker) readFileErr(p string) ([]byte, error) {
	if !w.visit(p) {
		err := w.s.missing[p]
		if err == nil {
			err = syscall.EIO
		}
		return nil, err
	}
	b, err := fs.ReadFile(w.fsys, p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, err
	case err != nil:
		w.add(Entry{Kind: Error, Path: p, Arg: errnoName(err)})
		return nil, err
	}
	w.add(Entry{Kind: File, Path: p})
	w.s.files[p] = b
	return b, nil
}

func (w walker) readDir(p string) []string {
	if !w.visit(p) {
		return nil
	}
	entries, err := fs.ReadDir(w.fsys, p)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			w.add(Entry{Kind: Error, Path: p, Arg: errnoName(err)})
		}
		return nil
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// errnoName is how replay gets the same error back; an error without an errno is recorded as EIO.
func errnoName(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		if name := unix.ErrnoName(errno); name != "" {
			return name
		}
	}
	return "EIO"
}

var networkFS = map[string]bool{"nfs": true, "nfs4": true, "cifs": true, "smb3": true, "smbfs": true, "ceph": true,
	"glusterfs": true, "afs": true, "9p": true, "lustre": true}

// skipMount says why a mount must never be statfs'd: autofs would trigger a mount, and a network or FUSE
// filesystem can block for as long as its server or daemon is away.
func skipMount(fstype string) string {
	switch {
	case fstype == "autofs":
		return SkipAutofs
	case networkFS[fstype]:
		return SkipNetwork
	case fstype == "fuse" || fstype == "fuseblk" || strings.HasPrefix(fstype, "fuse."):
		return SkipFuse
	}
	return ""
}

// statfsAll records statfs for every mount point in the snapshot's own mountinfo. The topmost entry for a path is
// what statfs would reach, so it alone decides: binfmt_misc mounted over its autofs trigger is statfs'd.
func (c *capturer) statfsAll(s *snapshot) {
	top := map[string]string{}
	ids := map[string]uint64{}
	var order []string
	sc := bufio.NewScanner(bytes.NewReader(s.files["proc/self/mountinfo"]))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		sep := slices.Index(f, "-")
		if sep < 5 || sep+1 >= len(f) {
			continue
		}
		mp := unescapeMount(f[4])
		if _, ok := top[mp]; !ok {
			order = append(order, mp)
		}
		top[mp] = f[sep+1]
		ids[mp], _ = strconv.ParseUint(f[0], 10, 64)
	}
	for _, mp := range order {
		if why := skipMount(top[mp]); why != "" {
			s.statfs[mp] = StatfsRecord{Skipped: why}
			s.skips = append(s.skips, mp+": "+top[mp]+" mount, never statfs'd")
			continue
		}
		blocked := false
		for ancestor, kind := range top {
			if ancestor != mp && strings.HasPrefix(mp, strings.TrimSuffix(ancestor, "/")+"/") && skipMount(kind) != "" {
				s.statfs[mp] = StatfsRecord{Skipped: skipMount(kind)}
				s.skips = append(s.skips, mp+": skipped ancestor mount")
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		s.statfs[mp] = c.statfs(mp, ids[mp])
		if r := s.statfs[mp]; r.Error != "" || r.Skipped != "" {
			s.skips = append(s.skips, mp+": "+r.Error+r.Skipped)
		}
	}
}

// A blocked kernel call cannot be cancelled; retaining it prevents another snapshot from duplicating it.
func (c *capturer) statfs(mp string, expected uint64) StatfsRecord {
	if c.pending == nil {
		c.pending = map[string]chan StatfsRecord{}
	}
	done, exists := c.pending[mp]
	if exists {
		select {
		case r := <-done:
			delete(c.pending, mp)
			if r.MntID == nil || *r.MntID != expected {
				return StatfsRecord{Skipped: "hidden/unverified"}
			}
			return r
		default:
			return StatfsRecord{Error: TimeoutError}
		}
	}
	done = make(chan StatfsRecord, 1)
	c.pending[mp] = done
	go func() {
		var r StatfsRecord
		if c.src.MountID != nil {
			id, err := c.src.MountID(mp)
			if err != nil || id != expected {
				r.Skipped = "hidden/unverified"
				done <- r
				return
			}
			r.MntID = &id
		}
		if st, err := c.src.Host.Statfs(mp); err != nil {
			r.Error = errnoName(err)
		} else {
			r.Stat = StatOf(st)
		}
		done <- r
	}()
	t := time.NewTimer(c.opt.StatfsTimeout)
	defer t.Stop()
	select {
	case r := <-done:
		delete(c.pending, mp)
		return r
	case <-t.C:
		return StatfsRecord{Error: TimeoutError}
	}
}

func unescapeMount(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// claim scrubs a source path and refuses a second source path that scrubs to the same one, which would silently
// merge two devices (two Bluetooth peripherals whose names differ only in their MAC, for example).
func (c *capturer) claim(raw string) (string, error) {
	scrubbed := c.scr.Quiet(raw)
	prev, ok := c.claimed[scrubbed]
	if ok && prev != raw {
		return "", fmt.Errorf("two paths scrub to %s; disconnect one of the devices and capture again", scrubbed)
	}
	if !ok {
		c.scr.Scrub(scrubbed+" (path)", []byte(raw))
		c.claimed[scrubbed] = raw
	}
	return scrubbed, nil
}

func (c *capturer) writeFile(raw string, data []byte) error {
	p := raw
	if dir := path.Dir(p); dir != "." {
		if err := c.root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return c.root.WriteFile(p, data, 0o644)
}

func (c *capturer) write(dir string, s *snapshot) error {
	index := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		raw := e.Path
		p, err := c.claim(dir + "/" + raw)
		if err != nil {
			return err
		}
		e.Path = strings.TrimPrefix(p, dir+"/")
		e.Arg = string(c.scr.Scrub(e.Path+" (target or error)", []byte(e.Arg)))
		index = append(index, e)
		if e.Kind == File {
			s.sizes[e.Path] = len(s.files[raw])
			if err := c.writeFile(p, c.scr.Scrub(e.Path, s.files[raw])); err != nil {
				return err
			}
		}
	}
	b, err := EncodeIndex(index)
	if err != nil {
		return err
	}
	s.entries = index
	if err := c.writeFile(dir+"/"+IndexFile, b); err != nil {
		return err
	}
	statfs := map[string]StatfsRecord{}
	for mp, r := range s.statfs {
		p := c.scr.Quiet(mp)
		if _, dup := statfs[p]; dup {
			return fmt.Errorf("two mount points scrub to %s", p)
		}
		r.Error = c.scr.Quiet(r.Error)
		statfs[p] = r
	}
	if b, err = json.MarshalIndent(statfs, "", "  "); err != nil {
		return err
	}
	return c.writeFile(dir+"/"+StatfsFile, append(b, '\n'))
}

func (c *capturer) report(w io.Writer, snaps []*snapshot) {
	q := c.scr.Quiet
	fmt.Fprintf(w, "Captured %d snapshots of this Machine as %q.\n\nSnapshot 0:\n", len(snaps), c.opt.Name)
	for _, e := range sortedEntries(snaps[0].entries) {
		switch e.Kind {
		case File:
			fmt.Fprintf(w, "  f %s (%d bytes)\n", e.Path, snaps[0].sizes[e.Path])
		case Dir:
			fmt.Fprintf(w, "  d %s\n", e.Path)
		default:
			suffix := ""
			if e.Dangling {
				suffix = " dangling"
			}
			fmt.Fprintf(w, "  %c %s -> %s%s\n", e.Kind, e.Path, q(e.Arg), suffix)
		}
	}
	first := indexSet(snaps[0].entries)
	for i, s := range snaps[1:] {
		other := indexSet(s.entries)
		var diff []string
		for k := range first {
			if !other[k] {
				diff = append(diff, "- "+k)
			}
		}
		for k := range other {
			if !first[k] {
				diff = append(diff, "+ "+k)
			}
		}
		slices.Sort(diff)
		fmt.Fprintf(w, "\nSnapshot %d, index differences from snapshot 0: %d\n", i+1, len(diff))
		for _, d := range diff {
			fmt.Fprintf(w, "  %s\n", q(d))
		}
	}
	fmt.Fprintln(w, "\nReplacements by rule:")
	counts := c.scr.Counts()
	if len(counts) == 0 {
		fmt.Fprintln(w, "  none")
	}
	for _, rule := range slices.Sorted(maps.Keys(counts)) {
		total := 0
		for _, n := range counts[rule] {
			total += n
		}
		fmt.Fprintf(w, "  %s: %d\n", rule, total)
		for _, f := range slices.Sorted(maps.Keys(counts[rule])) {
			fmt.Fprintf(w, "    %s: %d\n", f, counts[rule][f])
		}
	}
	for i, snap := range snaps {
		if i == 0 {
			fmt.Fprintf(w, "\nSkipped (snapshot %d):\n", i)
		} else {
			fmt.Fprintf(w, "\nSkip differences from snapshot 0 (snapshot %d):\n", i)
		}
		var differences []string
		if i == 0 {
			differences = snap.skips
		} else {
			for _, skip := range snap.skips {
				if !slices.Contains(snaps[0].skips, skip) {
					differences = append(differences, "+ "+skip)
				}
			}
			for _, skip := range snaps[0].skips {
				if !slices.Contains(snap.skips, skip) {
					differences = append(differences, "- "+skip)
				}
			}
		}
		if len(differences) == 0 {
			if i == 0 {
				fmt.Fprintln(w, "  none")
			} else {
				fmt.Fprintln(w, "  unchanged")
			}
		}
		for _, skip := range differences {
			fmt.Fprintf(w, "  %s\n", q(skip))
		}
	}
	fmt.Fprint(w, `
Next steps (contract/README.md § Captures):
  1. Read every file in the capture.
  2. Check that this Machine's hostname, MAC addresses, serial numbers and username appear nowhere in it.
  3. git add the capture, then run scripts/identifier-scan.sh --staged.
  4. Regenerate the fixtures: cd agent && go test ./internal/server -run '^TestGolden' -update
`)
}

func sortedEntries(entries []Entry) []Entry {
	b, err := EncodeIndex(entries)
	if err != nil {
		return entries
	}
	out, _ := DecodeIndex(b)
	return out
}

func indexSet(entries []Entry) map[string]bool {
	set := map[string]bool{}
	for _, e := range entries {
		if e.Kind != File {
			suffix := ""
			if e.Dangling {
				suffix = " dangling"
			}
			set[fmt.Sprintf("%c %s %s%s", e.Kind, e.Path, e.Arg, suffix)] = true
		} else {
			set["f "+e.Path] = true
		}
	}
	return set
}
