package qa

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DevicePort is where the app looks by default, so the Machine typed on the device is the bare synthetic address.
const DevicePort = 7247

// DefaultAddress is the synthetic tailnet address the app is pointed at. It exists only inside the emulator's
// routing; it is never bound or routed on the host, which is itself a tailnet node.
const DefaultAddress = "100.64.0.7"

const (
	deviceTmp    = "/data/local/tmp"
	ownerMarker  = deviceTmp + "/anshin-qa-owner"
	appDataDir   = "/data/data/" + AppID
	datastoreDir = appDataDir + "/files/datastore"
)

var tailnetV4 = netip.MustParsePrefix("100.64.0.0/10")

// UpOptions selects the device and route for one run.
type UpOptions struct {
	Run       string
	Device    string // the AVD name device_list reports, resolved to a serial
	Serial    string
	Route     string // "reverse" or "device"
	Address   string
	AgentArgs []string
}

// UpRecord is what up hands back: everything later operations and the report need.
type UpRecord struct {
	RunID      string       `json:"runId"`
	Serial     string       `json:"serial"`
	Route      string       `json:"route"`
	Address    string       `json:"address"`
	DevicePort int          `json:"devicePort"`
	APK        APKRecord    `json:"apk"`
	Seeds      []SeedRecord `json:"seeds"`
	SeedError  string       `json:"seedError,omitempty"`
	Probe      string       `json:"routeProbe"`
}

func (h *Helper) adbCmd(sdk SDK, serial string, args ...string) Cmd {
	if serial == "" {
		return Cmd{Name: sdk.ADB, Args: args}
	}
	return Cmd{Name: sdk.ADB, Args: append([]string{"-s", serial}, args...)}
}

func (h *Helper) adb(ctx context.Context, sdk SDK, serial string, args ...string) (string, error) {
	c := h.adbCmd(sdk, serial, args...)
	out, errb, err := h.run(ctx, c)
	if err != nil {
		return strings.TrimSpace(string(out)), cmdError(c, errb, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (h *Helper) shell(ctx context.Context, sdk SDK, serial, script string) (string, error) {
	return h.adb(ctx, sdk, serial, "shell", script)
}

// resolveSerial finds the running emulator whose AVD is name. The helper only ever works on emulators: it roots
// the device and changes its routing, which it must never do to a phone.
func (h *Helper) resolveSerial(ctx context.Context, sdk SDK, name, serial string) (string, error) {
	if serial != "" {
		if !strings.HasPrefix(serial, "emulator-") {
			return "", fmt.Errorf("%q is not an emulator; the helper roots and reroutes its device, so it refuses anything else", serial)
		}
		return serial, nil
	}
	if name == "" {
		return "", errors.New("--device (the AVD name from device_list) or --serial is required")
	}
	out, err := h.adb(ctx, sdk, "", "devices")
	if err != nil {
		return "", err
	}
	var found []string
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || f[1] != "device" || !strings.HasPrefix(f[0], "emulator-") {
			continue
		}
		avd, err := h.adb(ctx, sdk, f[0], "emu", "avd", "name")
		if err == nil && strings.TrimSpace(strings.SplitN(avd, "\n", 2)[0]) == name {
			found = append(found, f[0])
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no booted emulator runs AVD %q; open it with device_open first", name)
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("AVD %q runs on several emulators (%v); pass --serial", name, found)
}

// Up builds and signs the release APK, then takes ownership of the emulator's plumbing for this run: root, the app
// install, the QA Agent and the route to it. Each resource is in the ledger before it exists, so `down` can undo a
// run that stopped half way. Up never boots, opens or drives the device: the coordinator's device tools do.
func (h *Helper) Up(ctx context.Context, o UpOptions) (UpRecord, error) {
	unlock, lockErr := lockRun(o.Run)
	if lockErr != nil {
		return UpRecord{}, lockErr
	}
	defer unlock()
	rec := UpRecord{Route: o.Route, Address: o.Address, DevicePort: DevicePort}
	info, err := loadRun(o.Run)
	if err != nil {
		return rec, err
	}
	rec.RunID = info.RunID
	h.useRun(o.Run)
	l, err := loadLedger(o.Run, info.RunID)
	if err != nil {
		return rec, err
	}
	for _, r := range l.Resources {
		if r.State != stateRemoved {
			return rec, errors.New("this run still owns resources from an earlier up; run down first")
		}
	}
	fail := func(err error) (UpRecord, error) {
		l.fail("up: " + err.Error())
		return rec, err
	}
	if rec.Route == "" {
		rec.Route = "reverse"
	}
	if rec.Route != "reverse" && rec.Route != "device" {
		return fail(fmt.Errorf("--route %q: want reverse or device", rec.Route))
	}
	if rec.Address == "" {
		rec.Address = DefaultAddress
	}
	addr, err := netip.ParseAddr(rec.Address)
	if err != nil || !addr.Is4() || !tailnetV4.Contains(addr) || addr == netip.MustParseAddr("100.100.100.100") {
		return fail(fmt.Errorf("--address %q must be an IPv4 address in 100.64.0.0/10 other than 100.100.100.100", rec.Address))
	}
	if _, _, err := ParseAgentFlags(append(o.AgentArgs, "-listen", "127.0.0.1:1"), io.Discard); err != nil {
		return fail(fmt.Errorf("QA Agent flags: %w", err))
	}

	sha, clean, err := h.headState(ctx)
	if err != nil {
		return fail(err)
	}
	if sha != info.TargetSHA || !clean {
		return fail(fmt.Errorf("the checkout is at %s (clean: %v) but the run targets %s; a device run builds only the target SHA from a clean tree", sha, clean, info.TargetSHA))
	}
	sdk, err := findSDK(h.Repo, h.Getenv)
	if err != nil {
		return fail(err)
	}
	if rec.APK, err = h.buildRelease(ctx, o.Run, info, sdk); err != nil {
		return fail(fmt.Errorf("release build: %w", err))
	}
	if rec.Seeds, err = h.generateSeeds(ctx, o.Run); err != nil {
		// Theme cells need seeds; the rest of the run doesn't, so the gap is recorded rather than fatal.
		rec.SeedError = err.Error()
	}
	if err := h.buildAgent(ctx, o.Run, "", ""); err != nil {
		return fail(err)
	}
	if sha2, clean2, err := h.headState(ctx); err != nil || sha2 != info.TargetSHA || !clean2 {
		return fail(errors.New("the checkout changed while building; the APK no longer matches one clean SHA"))
	}

	serial, err := h.resolveSerial(ctx, sdk, o.Device, o.Serial)
	if err != nil {
		return fail(err)
	}
	rec.Serial = serial
	l.Serial, l.Route = serial, rec.Route
	if err := l.save(); err != nil {
		return rec, err
	}
	if err := h.claimDevice(ctx, sdk, l, serial); err != nil {
		return fail(err)
	}
	if err := h.install(ctx, sdk, l, serial, rec.APK.Path); err != nil {
		return fail(err)
	}
	if rec.Route == "reverse" {
		err = h.routeReverse(ctx, sdk, l, o.Run, serial, rec.Address, o.AgentArgs)
	} else {
		err = h.routeDevice(ctx, sdk, l, o.Run, serial, rec.Address, o.AgentArgs)
	}
	if err != nil {
		return fail(err)
	}
	rec.Probe = h.probe(ctx, sdk, serial, rec.Address)
	return rec, writeJSON(filepath.Join(o.Run, "up.json"), rec)
}

func (h *Helper) buildAgent(ctx context.Context, run, goos, goarch string) error {
	out := filepath.Join(run, "bin", "anshin-qa")
	env := []string{"CGO_ENABLED=0"}
	if goos != "" {
		out += "-" + goos + "-" + goarch
		env = append(env, "GOOS="+goos, "GOARCH="+goarch)
	}
	c := Cmd{Name: "go", Args: []string{"build", "-trimpath", "-o", out, "./cmd/anshin-qa"}, Dir: filepath.Join(h.Repo, "agent"), Env: env}
	if _, errb, err := h.run(ctx, c); err != nil {
		return cmdError(c, errb, err)
	}
	return nil
}

// claimDevice roots the emulator, marks it as this run's and snapshots the settings a run changes.
func (h *Helper) claimDevice(ctx context.Context, sdk SDK, l *Ledger, serial string) error {
	if state, err := h.adb(ctx, sdk, serial, "get-state"); err != nil || state != "device" {
		return fmt.Errorf("emulator %s is not ready (state %q): %v", serial, state, err)
	}

	r, err := l.begin(kindMarker, map[string]string{"path": ownerMarker})
	if err != nil {
		return err
	}
	// Noclobber claims the marker in the device filesystem before any shared mutation.
	if _, err := h.shell(ctx, sdk, serial, "set -C; echo "+l.RunID+" > "+ownerMarker); err != nil {
		return fmt.Errorf("device claim failed (another run may own it): %w", err)
	}
	if err := l.set(r, stateActive, nil); err != nil {
		return err
	}
	if uid, err := h.shell(ctx, sdk, serial, "id -u"); err != nil {
		return err
	} else if uid != "0" {
		r, err := l.begin(kindRoot, map[string]string{"serial": serial})
		if err != nil {
			return err
		}
		if _, err := h.adb(ctx, sdk, serial, "root"); err != nil {
			return fmt.Errorf("adb root (needs a non-Play emulator image): %w", err)
		}
		if _, err := h.adb(ctx, sdk, serial, "wait-for-device"); err != nil {
			return err
		}
		if uid, err := h.shell(ctx, sdk, serial, "id -u"); err != nil {
			return err
		} else if uid != "0" {
			return errors.New("adb root did not give a root shell; the image may be a Play image")
		}
		l.set(r, stateActive, nil)
	}
	font, err := h.shell(ctx, sdk, serial, "settings get system font_scale")
	if err != nil {
		return err
	}
	night, err := h.shell(ctx, sdk, serial, "cmd uimode night")
	if err != nil {
		return err
	}
	r, err = l.begin(kindSettings, map[string]string{"font_scale": font, "night": parseNight(night)})
	if err != nil {
		return err
	}
	return l.set(r, stateActive, nil)
}

func parseNight(out string) string {
	for _, v := range []string{"yes", "no", "auto", "custom"} {
		if strings.Contains(out, "Night mode: "+v) {
			return v
		}
	}
	return ""
}

// install installs the QA APK only when the app is absent. An existing install may hold the Owner's data and is
// signed with another key, so the run refuses it instead of uninstalling it.
func (h *Helper) install(ctx context.Context, sdk SDK, l *Ledger, serial, apk string) error {
	if out, err := h.shell(ctx, sdk, serial, "pm list packages "+AppID); err != nil {
		return err
	} else if strings.Contains(out, "package:"+AppID) {
		return fmt.Errorf("%s is already installed and this run did not install it; uninstalling it would delete its data, so remove it yourself or use another emulator", AppID)
	}
	r, err := l.begin(kindInstall, map[string]string{"package": AppID, "prior": "absent"})
	if err != nil {
		return err
	}
	if _, err := h.adb(ctx, sdk, serial, "install", apk); err != nil {
		return err
	}
	identity, err := h.installedIdentity(ctx, sdk, serial)
	if err != nil {
		return err
	}
	r.Data["identity"] = identity
	return l.set(r, stateActive, nil)
}

func (h *Helper) installedIdentity(ctx context.Context, sdk SDK, serial string) (string, error) {
	out, err := h.shell(ctx, sdk, serial, "pm list packages "+AppID)
	if err != nil {
		return "", err
	}
	if !strings.Contains(out, "package:"+AppID) {
		return "", nil
	}
	path, err := h.shell(ctx, sdk, serial, "pm path "+AppID)
	if err != nil {
		return "", err
	}
	path = strings.TrimPrefix(strings.Split(path, "\n")[0], "package:")
	if !strings.HasPrefix(path, "/data/app/") || !regexp.MustCompile(`^/data/app/[A-Za-z0-9_./=+~-]+$`).MatchString(path) {
		return "", errors.New("invalid installed APK path")
	}
	digest, err := h.shell(ctx, sdk, serial, "sha256sum "+shellQuote(path))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(digest)
	if len(fields) == 0 || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(fields[0]) {
		return "", errors.New("invalid installed APK digest")
	}
	return path + ":" + fields[0], nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (h *Helper) requireOwner(ctx context.Context, sdk SDK, l *Ledger) error {
	owner, err := h.shell(ctx, sdk, l.Serial, "cat "+ownerMarker)
	if err != nil {
		return err
	}
	if owner != l.RunID {
		return fmt.Errorf("device ownership changed to %q", owner)
	}
	return nil
}

func (h *Helper) requireInstall(ctx context.Context, sdk SDK, l *Ledger, r *Resource) error {
	if err := h.requireOwner(ctx, sdk, l); err != nil {
		return err
	}
	identity, err := h.installedIdentity(ctx, sdk, l.Serial)
	if err != nil {
		return err
	}
	if identity == "" {
		return nil
	}
	if r.Data["identity"] == "" || identity != r.Data["identity"] {
		return errors.New("installed APK identity changed or was not recorded")
	}
	return nil
}

func (h *Helper) dnatExists(ctx context.Context, sdk SDK, serial, address string) (bool, error) {
	out, err := h.shell(ctx, sdk, serial, "iptables -t nat -S OUTPUT")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		want := map[string]string{"-A": "OUTPUT", "-p": "tcp", "-d": address, "--dport": strconv.Itoa(DevicePort), "-j": "DNAT", "--to-destination": "127.0.0.1:" + strconv.Itoa(DevicePort)}
		matched := map[string]bool{}
		valid := len(fields)%2 == 0
		for i := 0; i+1 < len(fields); i += 2 {
			key, value := fields[i], fields[i+1]
			if key == "-m" && value == "tcp" {
				continue
			}
			if key == "-d" {
				value = strings.TrimSuffix(value, "/32")
			}
			if expected, ok := want[key]; !ok || expected != value || matched[key] {
				valid = false
			}
			matched[key] = true
		}
		if valid && len(matched) == len(want) {
			return true, nil
		}
	}
	return false, nil
}

func freeLoopbackPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func dnatArgs(op, address string) string {
	return fmt.Sprintf("iptables -t nat %s OUTPUT -p tcp -d %s --dport %d -j DNAT --to-destination 127.0.0.1:%d", op, address, DevicePort, DevicePort)
}

// routeReverse is the first route: the QA Agent on the host's loopback, adb reverse onto the emulator's loopback,
// and a DNAT rule in the emulator from the synthetic tailnet address to it. The app's destination check sees a
// tailnet address and is not changed.
func (h *Helper) routeReverse(ctx context.Context, sdk SDK, l *Ledger, run, serial, address string, agentArgs []string) error {
	port, err := freeLoopbackPort()
	if err != nil {
		return err
	}
	if err := h.startHostAgent(ctx, l, run, port, agentArgs); err != nil {
		return err
	}
	list, err := h.adb(ctx, sdk, serial, "reverse", "--list")
	if err != nil {
		return err
	}
	if strings.Contains(list, fmt.Sprintf("tcp:%d ", DevicePort)) {
		return fmt.Errorf("the emulator already reverses tcp:%d for someone else", DevicePort)
	}
	r, err := l.begin(kindReverse, map[string]string{"remote": fmt.Sprintf("tcp:%d", DevicePort), "local": fmt.Sprintf("tcp:%d", port)})
	if err != nil {
		return err
	}
	if _, err := h.adb(ctx, sdk, serial, "reverse", fmt.Sprintf("tcp:%d", DevicePort), fmt.Sprintf("tcp:%d", port)); err != nil {
		return err
	}
	l.set(r, stateActive, nil)
	if exists, err := h.dnatExists(ctx, sdk, serial, address); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("a DNAT rule for %s already exists and this run did not add it", address)
	}
	r, err = l.begin(kindDNAT, map[string]string{"address": address})
	if err != nil {
		return err
	}
	if _, err := h.shell(ctx, sdk, serial, dnatArgs("-A", address)); err != nil {
		return err
	}
	return l.set(r, stateActive, nil)
}

// routeDevice is the second route: the QA Agent cross-compiled and run inside the emulator, bound to the synthetic
// address added to its loopback.
func (h *Helper) routeDevice(ctx context.Context, sdk SDK, l *Ledger, run, serial, address string, agentArgs []string) error {
	abi, err := h.shell(ctx, sdk, serial, "getprop ro.product.cpu.abi")
	if err != nil {
		return err
	}
	goarch := map[string]string{"x86_64": "amd64", "arm64-v8a": "arm64"}[abi]
	if goarch == "" {
		return fmt.Errorf("no QA Agent build for ABI %q", abi)
	}
	if err := h.buildAgent(ctx, run, "linux", goarch); err != nil {
		return err
	}
	dir := deviceTmp + "/anshin-qa-" + l.RunID
	r, err := l.begin(kindDeviceDir, map[string]string{"path": dir})
	if err != nil {
		return err
	}
	if _, err := h.shell(ctx, sdk, serial, "mkdir -p "+dir); err != nil {
		return err
	}
	if _, err := h.adb(ctx, sdk, serial, "push", filepath.Join(run, "bin", "anshin-qa-linux-"+goarch), dir+"/anshin-qa"); err != nil {
		return err
	}
	if _, err := h.shell(ctx, sdk, serial, "chmod 755 "+dir+"/anshin-qa"); err != nil {
		return err
	}
	l.set(r, stateActive, nil)
	if out, err := h.shell(ctx, sdk, serial, "ip -4 addr show dev lo"); err != nil {
		return err
	} else if strings.Contains(out, " "+address+"/") {
		return fmt.Errorf("%s is already on the emulator's loopback and this run did not add it", address)
	}
	r, err = l.begin(kindAlias, map[string]string{"address": address})
	if err != nil {
		return err
	}
	if _, err := h.shell(ctx, sdk, serial, "ip addr add "+address+"/32 dev lo"); err != nil {
		return err
	}
	l.set(r, stateActive, nil)
	return h.startDeviceAgent(ctx, sdk, l, run, serial, address, agentArgs)
}

// probe asks the route for a Pulse from the emulator's root shell, which passes through the same OUTPUT chain the app
// does. It only hints: the app's own Live screen and the access log are the evidence.
func (h *Helper) probe(ctx context.Context, sdk SDK, serial, address string) string {
	script := fmt.Sprintf("printf 'GET /v1/pulse HTTP/1.0\\r\\n\\r\\n' | toybox nc -w 3 %s %d 2>&1 | head -n 1", address, DevicePort)
	out, err := h.shell(ctx, sdk, serial, script)
	switch {
	case err != nil:
		return "unavailable: " + err.Error()
	case strings.HasPrefix(out, "HTTP/1."):
		return "answered: " + out
	}
	return "no answer: " + out
}

var safeArg = regexp.MustCompile(`^[A-Za-z0-9_.:/=+-]+$`)

// agentArgs validates the QA Agent's setup, copies a raw body into the run so its exact bytes are evidence, and
// returns the arguments with the body's path as the QA Agent will see it.
func (h *Helper) agentArgs(run string, args []string, listen string, bodyDir string) ([]string, map[string]string, error) {
	if _, _, err := ParseAgentFlags(append(append([]string{}, args...), "-listen", listen), io.Discard); err != nil {
		return nil, nil, err
	}
	meta := map[string]string{"args": strings.Join(args, " ")}
	out := make([]string, 0, len(args)+2)
	for i := 0; i < len(args); i++ {
		a := args[i]
		body, isBody := "", false
		switch {
		case a == "-body" || a == "--body":
			if i+1 >= len(args) {
				return nil, nil, errors.New("-body needs a file")
			}
			body, isBody = args[i+1], true
			i++
		case strings.HasPrefix(a, "-body=") || strings.HasPrefix(a, "--body="):
			body, isBody = a[strings.IndexByte(a, '=')+1:], true
		}
		if !isBody {
			out = append(out, a)
			continue
		}
		sum, _, err := fileDigest(body)
		if err != nil {
			return nil, nil, err
		}
		local := filepath.Join(run, "bodies", sum[:12]+".body")
		if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
			return nil, nil, err
		}
		if err := copyFile(body, local); err != nil {
			return nil, nil, err
		}
		meta["body"], meta["bodySha256"], meta["bodySource"] = local, sum, filepath.Base(body)
		target := local
		if bodyDir != "" {
			target = bodyDir + "/" + filepath.Base(local)
		}
		out = append(out, "-body", target)
	}
	for _, a := range out {
		if !safeArg.MatchString(a) {
			return nil, nil, fmt.Errorf("QA Agent argument %q has characters the helper won't pass through a shell", a)
		}
	}
	return append(out, "-listen", listen), meta, nil
}

func (h *Helper) startHostAgent(ctx context.Context, l *Ledger, run string, port int, args []string) error {
	listen := fmt.Sprintf("127.0.0.1:%d", port)
	full, meta, err := h.agentArgs(run, args, listen, "")
	if err != nil {
		return err
	}
	n := len(l.Resources) + 1
	logs := filepath.Join(run, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		return err
	}
	meta["port"], meta["access"], meta["stderr"] = strconv.Itoa(port), filepath.Join(logs, fmt.Sprintf("access-%d.jsonl", n)), filepath.Join(logs, fmt.Sprintf("agent-%d.log", n))
	r, err := l.begin(kindHostAgent, meta)
	if err != nil {
		return err
	}
	c := Cmd{Name: filepath.Join(run, "bin", "anshin-qa"), Args: full}
	pid, st, err := h.Procs.Start(c, meta["access"], meta["stderr"])
	if err != nil {
		return err
	}
	r.Data["pid"], r.Data["startTime"], r.Data["command"] = strconv.Itoa(pid), st, c.String()
	l.save()
	for range 50 {
		if !h.Procs.Alive(pid, st) {
			return fmt.Errorf("the QA Agent exited at start; see %s", meta["stderr"])
		}
		if conn, err := net.DialTimeout("tcp", listen, 100*time.Millisecond); err == nil {
			conn.Close()
			return l.set(r, stateActive, nil)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("the QA Agent did not listen on %s within 5s", listen)
}

func (h *Helper) startDeviceAgent(ctx context.Context, sdk SDK, l *Ledger, run, serial, address string, args []string) error {
	dir := deviceTmp + "/anshin-qa-" + l.RunID
	full, meta, err := h.agentArgs(run, args, fmt.Sprintf("%s:%d", address, DevicePort), dir)
	if err != nil {
		return err
	}
	if meta["body"] != "" {
		if _, err := h.adb(ctx, sdk, serial, "push", meta["body"], dir+"/"+filepath.Base(meta["body"])); err != nil {
			return err
		}
	}
	n := len(l.Resources) + 1
	meta["access"], meta["stderr"], meta["dir"] = fmt.Sprintf("%s/access-%d.jsonl", dir, n), fmt.Sprintf("%s/agent-%d.log", dir, n), dir
	r, err := l.begin(kindDeviceAgent, meta)
	if err != nil {
		return err
	}
	script := fmt.Sprintf("cd %s && (nohup ./anshin-qa %s > %s 2> %s < /dev/null & echo $!)", dir, strings.Join(full, " "), meta["access"], meta["stderr"])
	pid, err := h.shell(ctx, sdk, serial, script)
	if err != nil {
		return err
	}
	if _, err := strconv.Atoi(pid); err != nil {
		return fmt.Errorf("starting the QA Agent on the emulator printed %q, not a PID", pid)
	}
	r.Data["pid"] = pid
	if err := l.save(); err != nil {
		return err
	}
	identity, err := h.deviceProcessIdentity(ctx, sdk, serial, pid, dir)
	if err != nil {
		return err
	}
	r.Data["identity"] = identity
	l.save()
	time.Sleep(500 * time.Millisecond)
	if _, err := h.shell(ctx, sdk, serial, "kill -0 "+pid); err != nil {
		return fmt.Errorf("the QA Agent on the emulator exited at start; its log is %s on the device", meta["stderr"])
	}
	return l.set(r, stateActive, nil)
}

func (h *Helper) deviceProcessIdentity(ctx context.Context, sdk SDK, serial, pid, dir string) (string, error) {
	n, err := strconv.Atoi(pid)
	if err != nil || n <= 0 || strconv.Itoa(n) != pid || !strings.HasPrefix(dir, deviceTmp+"/anshin-qa-") || !safeArg.MatchString(dir) {
		return "", errors.New("unsafe device process identity")
	}
	script := "if [ ! -d /proc/" + pid + " ]; then echo absent; else cat /proc/" + pid + "/stat && readlink /proc/" + pid + "/exe && readlink /proc/" + pid + "/cwd; fi"
	out, err := h.shell(ctx, sdk, serial, script)
	if err != nil || out == "absent" {
		return out, err
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 3 || lines[1] != dir+"/anshin-qa" || lines[2] != dir {
		return "", errors.New("device process executable or cwd changed")
	}
	end := strings.LastIndex(lines[0], ")")
	if end < 0 {
		return "", errors.New("invalid process stat")
	}
	fields := strings.Fields(lines[0][end+1:])
	if len(fields) < 20 {
		return "", errors.New("missing process start time")
	}
	return fields[19] + ":" + lines[1] + ":" + lines[2], nil
}

func (h *Helper) stopHostAgent(r *Resource) error {
	pid, _ := strconv.Atoi(r.Data["pid"])
	if pid == 0 {
		return nil
	}
	return h.Procs.Stop(pid, r.Data["startTime"])
}

// stopDeviceAgent requires start time, executable and cwd to match so PID reuse cannot kill another run.
func (h *Helper) stopDeviceAgent(ctx context.Context, sdk SDK, run, serial string, r *Resource) error {
	if pid := r.Data["pid"]; pid != "" {
		identity, err := h.deviceProcessIdentity(ctx, sdk, serial, pid, r.Data["dir"])
		if err != nil {
			return err
		}
		if identity != "absent" {
			if r.Data["identity"] == "" || identity != r.Data["identity"] {
				return errors.New("device PID identity changed or missing")
			}
			if _, err := h.shell(ctx, sdk, serial, "kill "+pid); err != nil {
				return err
			}
		}
	}
	logs := filepath.Join(run, "logs")
	os.MkdirAll(logs, 0o700)
	for _, k := range []string{"access", "stderr"} {
		if p := r.Data[k]; p != "" {
			h.adb(ctx, sdk, serial, "pull", p, filepath.Join(logs, filepath.Base(p)))
		}
	}
	return nil
}

func (h *Helper) openRun(run string) (RunInfo, *Ledger, error) {
	info, err := loadRun(run)
	if err != nil {
		return info, nil, err
	}
	h.useRun(run)
	l, err := loadLedger(run, info.RunID)
	return info, l, err
}

// AgentStop stops the run's QA Agent and leaves the route, so the phone sees an Agent that went away.
func (h *Helper) AgentStop(ctx context.Context, run string) error {
	unlock, lockErr := lockRun(run)
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	_, l, err := h.openRun(run)
	if err != nil {
		return err
	}
	return h.agentStop(ctx, l, run)
}

func (h *Helper) agentStop(ctx context.Context, l *Ledger, run string) error {
	var sdk SDK
	for _, r := range append(l.live(kindHostAgent), l.live(kindDeviceAgent)...) {
		var err error
		if r.Kind == kindHostAgent {
			err = h.stopHostAgent(r)
		} else {
			if sdk.ADB == "" {
				if sdk, err = findSDK(h.Repo, h.Getenv); err != nil {
					return err
				}
			}
			err = h.stopDeviceAgent(ctx, sdk, run, l.Serial, r)
		}
		if err != nil {
			l.set(r, stateFailed, err)
			return err
		}
		l.set(r, stateRemoved, nil)
	}
	return nil
}

// AgentStart restarts the run's QA Agent with new setup flags on the route up made.
func (h *Helper) AgentStart(ctx context.Context, run string, args []string) error {
	unlock, lockErr := lockRun(run)
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	_, l, err := h.openRun(run)
	if err != nil {
		return err
	}
	if l.Route == "" || (!l.owns(kindReverse) && !l.owns(kindAlias)) {
		return errors.New("this run has no route; run up first")
	}
	if err := h.agentStop(ctx, l, run); err != nil {
		return err
	}
	if l.Route == "reverse" {
		port := ""
		for _, r := range l.live(kindReverse) {
			port = strings.TrimPrefix(r.Data["local"], "tcp:")
		}
		p, err := strconv.Atoi(port)
		if err != nil {
			return errors.New("the ledger's adb reverse has no host port")
		}
		return h.startHostAgent(ctx, l, run, p, args)
	}
	sdk, err := findSDK(h.Repo, h.Getenv)
	if err != nil {
		return err
	}
	var address string
	for _, r := range l.live(kindAlias) {
		address = r.Data["address"]
	}
	return h.startDeviceAgent(ctx, sdk, l, run, l.Serial, address, args)
}

// Seed puts one theme's generated preference file in place while the app is stopped. It is install-time plumbing
// for a release app the run installed; it adds nothing to the app.
func (h *Helper) Seed(ctx context.Context, run, theme string) (SeedRecord, error) {
	unlock, lockErr := lockRun(run)
	if lockErr != nil {
		return SeedRecord{}, lockErr
	}
	defer unlock()
	var rec SeedRecord
	_, l, err := h.openRun(run)
	if err != nil {
		return rec, err
	}
	if !l.owns(kindInstall) {
		return rec, errors.New("this run did not install the app, so it won't write the app's data")
	}
	var seeds []SeedRecord
	if err := readJSON(filepath.Join(run, "seeds", "seeds.json"), &seeds); err != nil {
		return rec, fmt.Errorf("no generated seeds (up records why in up.json seedError): %w", err)
	}
	for _, s := range seeds {
		if s.Theme == theme {
			rec = s
		}
	}
	if rec.Theme == "" {
		return rec, fmt.Errorf("no seed for theme %q; themes are %v", theme, ThemeKeys)
	}
	if sum, _, err := fileDigest(rec.Path); err != nil || sum != rec.SHA256 {
		return rec, fmt.Errorf("seed %s changed since up generated it", rec.Path)
	}
	sdk, err := findSDK(h.Repo, h.Getenv)
	if err != nil {
		return rec, err
	}
	for _, installed := range l.live(kindInstall) {
		if err := h.requireInstall(ctx, sdk, l, installed); err != nil {
			return rec, err
		}
	}
	if identity, err := h.installedIdentity(ctx, sdk, l.Serial); err != nil {
		return rec, err
	} else if identity == "" {
		return rec, errors.New("the run's installed app is absent")
	}
	if names, err := h.shell(ctx, sdk, l.Serial, "ps -A -o NAME"); err != nil {
		return rec, err
	} else if slices.Contains(strings.Fields(names), AppID) {
		return rec, errors.New("the app is running; close it through the bound agent-device command, then seed")
	}
	owner, err := h.shell(ctx, sdk, l.Serial, "stat -c %u:%g "+appDataDir)
	if err != nil || !regexp.MustCompile(`^\d+:\d+$`).MatchString(owner) {
		return rec, fmt.Errorf("reading the app's data owner: %q %v", owner, err)
	}
	r, err := l.begin(kindSeed, map[string]string{"theme": theme, "sha256": rec.SHA256})
	if err != nil {
		return rec, err
	}
	tmp := deviceTmp + "/anshin-qa-seed-" + l.RunID
	if _, err := h.adb(ctx, sdk, l.Serial, "push", rec.Path, tmp); err != nil {
		l.set(r, stateFailed, err)
		return rec, err
	}
	files := appDataDir + "/files"
	script := strings.Join([]string{
		"set -e",
		fmt.Sprintf("[ -d %[1]s ] || { mkdir %[1]s; chown %[2]s %[1]s; chmod 771 %[1]s; }", files, owner),
		fmt.Sprintf("[ -d %[1]s ] || { mkdir %[1]s; chown %[2]s %[1]s; chmod 700 %[1]s; }", datastoreDir, owner),
		fmt.Sprintf("cp %s %s/.settings.qa", tmp, datastoreDir),
		fmt.Sprintf("chown %s %s/.settings.qa", owner, datastoreDir),
		fmt.Sprintf("chmod 600 %s/.settings.qa", datastoreDir),
		fmt.Sprintf("mv %[1]s/.settings.qa %[1]s/settings.preferences_pb", datastoreDir),
		"restorecon -R " + files,
		"rm -f " + tmp,
		fmt.Sprintf("sha256sum %s/settings.preferences_pb", datastoreDir),
	}, "; ")
	out, err := h.shell(ctx, sdk, l.Serial, script)
	if err != nil {
		l.set(r, stateFailed, err)
		return rec, err
	}
	if f := strings.Fields(out); len(f) == 0 || f[0] != rec.SHA256 {
		err := fmt.Errorf("the seeded file's digest is %q, want %s", out, rec.SHA256)
		l.set(r, stateFailed, err)
		return rec, err
	}
	l.set(r, stateActive, nil)
	line := fmt.Sprintf(`{"time":%q,"theme":%q,"sha256":%q}`+"\n", h.Now().UTC().Format(time.RFC3339), theme, rec.SHA256)
	f, err := os.OpenFile(filepath.Join(run, "seeds-applied.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		f.WriteString(line)
		f.Close()
	}
	return rec, nil
}

// CleanupRecord is down's result. Failed lists every resource that may still exist, with why.
type CleanupRecord struct {
	Status   string      `json:"status"`
	Finished time.Time   `json:"finished"`
	Removed  []string    `json:"removed"`
	Failed   []string    `json:"failed"`
	Ledger   []*Resource `json:"ledger"`
}

// Down removes what the ledger says this run owns, newest first, and nothing else. It is safe to run again, and after
// an up that stopped half way. It needs adb only if the run touched the device.
func (h *Helper) Down(ctx context.Context, run string) (CleanupRecord, error) {
	unlock, lockErr := lockRun(run)
	if lockErr != nil {
		return CleanupRecord{}, lockErr
	}
	defer unlock()
	rec := CleanupRecord{Status: "ok"}
	_, l, err := h.openRun(run)
	if err != nil {
		return rec, err
	}
	var sdk SDK
	var sdkErr error
	needSDK := func() error {
		if sdk.ADB == "" && sdkErr == nil {
			sdk, sdkErr = findSDK(h.Repo, h.Getenv)
			if sdkErr == nil && !isExecutable(sdk.ADB) {
				sdkErr = errors.New("no adb in the Android SDK")
			}
		}
		return sdkErr
	}
	resources := slices.Clone(l.Resources)
	priority := func(r *Resource) int {
		if r.Kind == kindMarker {
			return 0
		}
		if r.Kind == kindRoot {
			return 1
		}
		return 2
	}
	sort.SliceStable(resources, func(i, j int) bool { return priority(resources[i]) < priority(resources[j]) })
	for i := len(resources) - 1; i >= 0; i-- {
		r := resources[i]
		if r.State == stateRemoved {
			continue
		}
		err := func() error {
			if len(rec.Failed) > 0 && (r.Kind == kindMarker || r.Kind == kindRoot || r.Kind == kindDeviceDir || r.Kind == kindAlias) {
				return errors.New("dependent cleanup failed; retaining prerequisite")
			}
			if r.Kind == kindHostAgent {
				return h.stopHostAgent(r)
			}
			if err := needSDK(); err != nil {
				return err
			}
			return h.undo(ctx, sdk, l, run, r)
		}()
		label := fmt.Sprintf("%d %s %v", r.ID, r.Kind, r.Data)
		if err != nil {
			l.set(r, stateFailed, err)
			rec.Failed = append(rec.Failed, label+": "+err.Error())
			continue
		}
		l.set(r, stateRemoved, nil)
		rec.Removed = append(rec.Removed, label)
	}
	if len(rec.Failed) > 0 {
		rec.Status = "failed"
	}
	rec.Finished, rec.Ledger = h.Now().UTC(), l.Resources
	if err := writeJSON(filepath.Join(run, "cleanup.json"), rec); err != nil {
		return rec, err
	}
	if rec.Status != "ok" {
		return rec, fmt.Errorf("cleanup left %d resource(s): %s", len(rec.Failed), strings.Join(rec.Failed, "; "))
	}
	return rec, nil
}

func (h *Helper) undo(ctx context.Context, sdk SDK, l *Ledger, run string, r *Resource) error {
	s := l.Serial
	for _, key := range []string{"pid", "path", "dir", "access", "stderr", "address", "font_scale", "remote", "local"} {
		if v := r.Data[key]; v != "" && !safeArg.MatchString(v) {
			return fmt.Errorf("unsafe ledger %s", key)
		}
	}
	if r.Kind != kindMarker && r.Kind != kindSeed {
		if err := h.requireOwner(ctx, sdk, l); err != nil {
			return err
		}
	}
	switch r.Kind {
	case kindSeed:
		return nil // removed with the app the run installed
	case kindDeviceAgent:
		return h.stopDeviceAgent(ctx, sdk, run, s, r)
	case kindAlias:
		if out, err := h.shell(ctx, sdk, s, "ip -4 addr show dev lo"); err != nil {
			return err
		} else if !strings.Contains(out, " "+r.Data["address"]+"/") {
			return nil
		}
		_, err := h.shell(ctx, sdk, s, "ip addr del "+r.Data["address"]+"/32 dev lo")
		return err
	case kindDeviceDir:
		dir := r.Data["path"]
		if dir != deviceTmp+"/anshin-qa-"+l.RunID {
			return fmt.Errorf("refusing to remove %q: not this run's directory", dir)
		}
		_, err := h.shell(ctx, sdk, s, "rm -rf "+dir)
		return err
	case kindDNAT:
		for range 5 {
			if exists, err := h.dnatExists(ctx, sdk, s, r.Data["address"]); err != nil {
				return err
			} else if !exists {
				return nil
			}
			if _, err := h.shell(ctx, sdk, s, dnatArgs("-D", r.Data["address"])); err != nil {
				return err
			}
		}
		return errors.New("the DNAT rule is still there after five deletions")
	case kindReverse:
		list, err := h.adb(ctx, sdk, s, "reverse", "--list")
		if err != nil {
			return err
		}
		if !strings.Contains(list, r.Data["remote"]+" "+r.Data["local"]) {
			return nil
		}
		_, err = h.adb(ctx, sdk, s, "reverse", "--remove", r.Data["remote"])
		return err
	case kindInstall:
		if err := h.requireInstall(ctx, sdk, l, r); err != nil {
			return err
		}
		identity, err := h.installedIdentity(ctx, sdk, s)
		if err != nil {
			return err
		}
		if identity == "" {
			return nil
		}
		h.shell(ctx, sdk, s, "am force-stop "+AppID)
		_, err = h.adb(ctx, sdk, s, "uninstall", AppID)
		return err
	case kindSettings:
		var errs []error
		if f := r.Data["font_scale"]; f == "null" || f == "" {
			_, err := h.shell(ctx, sdk, s, "settings delete system font_scale")
			errs = append(errs, err)
		} else {
			_, err := h.shell(ctx, sdk, s, "settings put system font_scale "+f)
			errs = append(errs, err)
		}
		if n := r.Data["night"]; n == "yes" || n == "no" || n == "auto" {
			_, err := h.shell(ctx, sdk, s, "cmd uimode night "+n)
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	case kindMarker:
		owner, err := h.shell(ctx, sdk, s, "cat "+ownerMarker)
		if err != nil {
			return err
		}
		if owner != l.RunID {
			return errors.New("device ownership changed")
		}
		_, err = h.shell(ctx, sdk, s, "rm -f "+ownerMarker)
		return err
	case kindRoot:
		_, err := h.adb(ctx, sdk, s, "unroot")
		return err
	}
	return fmt.Errorf("unknown resource kind %q", r.Kind)
}
