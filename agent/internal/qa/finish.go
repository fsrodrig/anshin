package qa

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// The plan, observation and proof files are written by the planner, the runner and the coordinator. finish reads
// them strictly enough that a typo becomes a listed problem, never a silent PASS.

type Plan struct {
	SchemaVersion     int            `json:"schemaVersion"`
	Scope             string         `json:"scope"`
	Scenarios         []PlanScenario `json:"scenarios"`
	OutsideQA         []PlanProof    `json:"outsideQA"`
	ManualCriteria    []PlanManual   `json:"manualCriteria"`
	CapabilityGaps    []PlanGap      `json:"capabilityGaps"`
	SpecGaps          []PlanGap      `json:"specGaps"`
	SpecDisagreements []PlanDisagree `json:"specDisagreements"`
}

type PlanScenario struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Required     *bool      `json:"required"`
	Cells        []PlanCell `json:"cells"`
	Capabilities []string   `json:"capabilities"`
	Gaps         []string   `json:"gaps"`
	Expected     string     `json:"expected"`
}

func (s PlanScenario) required() bool { return s.Required == nil || *s.Required }

type PlanCell struct {
	ID         string `json:"id"`
	State      string `json:"state"`
	Theme      string `json:"theme"`
	FontScale  string `json:"fontScale"`
	Appearance string `json:"appearance"`
}

type PlanProof struct {
	ID        string `json:"id"`
	Criterion string `json:"criterion"`
	Proof     string `json:"proof"`
}

type PlanManual struct {
	ID           string `json:"id"`
	Criterion    string `json:"criterion"`
	Confirmation string `json:"confirmation"`
}

type PlanGap struct {
	ID          string   `json:"id"`
	Summary     string   `json:"summary"`
	Blocks      []string `json:"blocks"`
	Consequence string   `json:"consequence"`
	Coverage    string   `json:"coverage"`
}

type PlanDisagree struct {
	ID             string `json:"id"`
	Spec           string `json:"spec"`
	Implementation string `json:"implementation"`
}

// Observation is one scenario as the runner saw it, in observations/<id>.json.
type Observation struct {
	ID     string   `json:"id"`
	Result string   `json:"result"`
	Cells  []Cell   `json:"cells"`
	Note   string   `json:"note"`
	Gaps   []string `json:"gaps"`
}

type Cell struct {
	ID             string   `json:"id"`
	Result         string   `json:"result"`
	Theme          string   `json:"theme"`
	FontMultiplier string   `json:"fontMultiplier"`
	Appearance     string   `json:"appearance"`
	Evidence       []string `json:"evidence"`
	Note           string   `json:"note"`
}

// Proofs is what the coordinator attaches for criteria proven outside QA and for manual confirmations.
type Proofs struct {
	OutsideQA      []ProofState  `json:"outsideQA"`
	ManualCriteria []ManualState `json:"manualCriteria"`
}

type ProofState struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	SHA     string `json:"sha"`
	Command string `json:"command"`
	Exit    *int   `json:"exit"`
	CIURL   string `json:"ciUrl"`
	Note    string `json:"note"`
}

type ManualState struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Evidence string `json:"evidence"`
}

// Result is result.json, schema version 1 of the QA return contract.
type Result struct {
	SchemaVersion     int              `json:"schemaVersion"`
	Result            string           `json:"result"`
	ReasonCode        string           `json:"reasonCode"`
	Reason            string           `json:"reason,omitempty"`
	Target            ResultTarget     `json:"target"`
	APK               *APKRecord       `json:"apk"`
	Scenarios         []ResultScenario `json:"scenarios"`
	OutsideQA         []ResultProof    `json:"outsideQA"`
	ManualCriteria    []ResultManual   `json:"manualCriteria"`
	CapabilityGaps    []PlanGap        `json:"capabilityGaps"`
	SpecGaps          []PlanGap        `json:"specGaps"`
	SpecDisagreements []PlanDisagree   `json:"specDisagreements"`
	Cleanup           ResultCleanup    `json:"cleanup"`
	Gates             Gates            `json:"gates"`
	Posting           Posting          `json:"posting"`
	Problems          []string         `json:"problems"`
	Finished          time.Time        `json:"finished"`
}

type ResultTarget struct {
	TestedSHA    string `json:"testedSha"`
	Branch       string `json:"branch"`
	MergeBase    string `json:"mergeBase"`
	PR           *int   `json:"pr"`
	CleanTree    bool   `json:"cleanTree"`
	HeadAtFinish string `json:"headAtFinish"`
	HeadMismatch bool   `json:"headMismatch"`
}

type ResultScenario struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Required bool     `json:"required"`
	Result   string   `json:"result"`
	Cells    []Cell   `json:"cells"`
	Gaps     []string `json:"gaps"`
	Note     string   `json:"note,omitempty"`
}

type ResultProof struct {
	ID        string `json:"id"`
	Criterion string `json:"criterion"`
	Proof     string `json:"proof"`
	State     string `json:"state"`
	SHA       string `json:"sha"`
	Command   string `json:"command"`
	Exit      *int   `json:"exit"`
	CIURL     string `json:"ciUrl"`
	Note      string `json:"note"`
}

type ResultManual struct {
	ID           string `json:"id"`
	Criterion    string `json:"criterion"`
	Confirmation string `json:"confirmation"`
	State        string `json:"state"`
	Evidence     string `json:"evidence"`
}

type ResultCleanup struct {
	Status string   `json:"status"`
	Failed []string `json:"failed"`
}

// Gates keeps the acceptance gates apart: a device PASS says nothing about CI, and a pending reviewer confirmation
// never becomes a device scenario.
type Gates struct {
	DeviceQA   string `json:"deviceQa"`
	OutsideQA  string `json:"outsideQa"`
	Manual     string `json:"manual"`
	Cleanup    string `json:"cleanup"`
	StageClean bool   `json:"stageClean"`
}

type Posting struct {
	Policy string  `json:"policy"`
	Posted bool    `json:"posted"`
	URL    *string `json:"url"`
}

type FinishOptions struct {
	Run     string
	Mode    string // "device", "planning" or "na"
	Reason  string
	Posting string // "none" (--no-post), "caller" or "authorized"
}

var (
	deviceResults = []string{"PASS", "FAIL", "INCONCLUSIVE", "NOT_RUN"}
	proofStates   = []string{"PASS", "FAIL", "PENDING", "NA"}
	manualStates  = []string{"CONFIRMED", "PENDING", "REJECTED"}
)

// Finish writes result.json, report.md (private), pr-summary.md (public-safe) and evidence-manifest.json. It changes
// nothing outside the run directory and posts nothing.
func (h *Helper) Finish(ctx context.Context, o FinishOptions) (Result, error) {
	unlock, lockErr := lockRun(o.Run)
	if lockErr != nil {
		return Result{}, lockErr
	}
	defer unlock()

	res := Result{SchemaVersion: 1, Posting: Posting{Policy: o.Posting}, Finished: h.Now().UTC()}
	info, err := loadRun(o.Run)
	if err != nil {
		return res, err
	}
	h.useRun(o.Run)
	if res.Posting.Policy == "" {
		res.Posting.Policy = "none"
	}
	problem := func(format string, a ...any) { res.Problems = append(res.Problems, fmt.Sprintf(format, a...)) }

	res.Target = ResultTarget{TestedSHA: info.TargetSHA, Branch: info.Branch, MergeBase: info.MergeBase, PR: info.PR, CleanTree: info.CleanTree}
	if head, _, err := h.headState(ctx); err == nil {
		res.Target.HeadAtFinish = head
		res.Target.HeadMismatch = head != info.TargetSHA
	}
	var apk APKRecord
	if readJSON(filepath.Join(o.Run, "build", "apk.json"), &apk) == nil {
		res.APK = &apk
	}

	var plan Plan
	if err := readJSON(filepath.Join(o.Run, "plan.json"), &plan); err != nil {
		problem("plan.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(o.Run, "plan.md")); err != nil {
		problem("plan.md is missing")
	}
	res.CapabilityGaps, res.SpecGaps, res.SpecDisagreements = plan.CapabilityGaps, plan.SpecGaps, plan.SpecDisagreements

	var proofs Proofs
	if err := readJSON(filepath.Join(o.Run, "proofs.json"), &proofs); err != nil && !errors.Is(err, fs.ErrNotExist) {
		problem("proofs.json: %v", err)
	}
	for _, p := range plan.OutsideQA {
		rp := ResultProof{ID: p.ID, Criterion: p.Criterion, Proof: p.Proof, State: "PENDING"}
		for _, s := range proofs.OutsideQA {
			if s.ID != p.ID {
				continue
			}
			rp.State, rp.SHA, rp.Command, rp.Exit, rp.CIURL, rp.Note = s.State, s.SHA, s.Command, s.Exit, s.CIURL, s.Note
			if !slices.Contains(proofStates, s.State) {
				problem("outsideQA %s: state %q is not one of %v", p.ID, s.State, proofStates)
				rp.State = "PENDING"
			}
			if rp.State == "PASS" && s.SHA != info.TargetSHA {
				problem("outsideQA %s: PASS is for %q, not the tested SHA; kept PENDING", p.ID, s.SHA)
				rp.State = "PENDING"
			}
		}
		res.OutsideQA = append(res.OutsideQA, rp)
	}
	for _, m := range plan.ManualCriteria {
		rm := ResultManual{ID: m.ID, Criterion: m.Criterion, Confirmation: m.Confirmation, State: "PENDING"}
		for _, s := range proofs.ManualCriteria {
			if s.ID == m.ID {
				rm.State, rm.Evidence = s.State, s.Evidence
				if !slices.Contains(manualStates, s.State) {
					problem("manual %s: state %q is not one of %v", m.ID, s.State, manualStates)
					rm.State = "PENDING"
				}
			}
		}
		res.ManualCriteria = append(res.ManualCriteria, rm)
	}

	l, lerr := loadLedger(o.Run, info.RunID)
	if lerr != nil {
		problem("ledger: %v", lerr)
		l = &Ledger{}
	}
	res.Cleanup = cleanupState(o.Run, l)

	obs := map[string]Observation{}
	entries, _ := os.ReadDir(filepath.Join(o.Run, "observations"))
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var ob Observation
		if err := readJSON(filepath.Join(o.Run, "observations", e.Name()), &ob); err != nil {
			problem("observation %s: %v", e.Name(), err)
			continue
		}
		if !slices.ContainsFunc(plan.Scenarios, func(s PlanScenario) bool { return s.ID == ob.ID }) {
			problem("observation %s names scenario %q, which the plan doesn't have", e.Name(), ob.ID)
			continue
		}
		obs[ob.ID] = ob
	}

	for _, s := range plan.Scenarios {
		rs := ResultScenario{ID: s.ID, Title: s.Title, Required: s.required(), Gaps: s.Gaps, Result: "NOT_RUN"}
		if ob, ok := obs[s.ID]; ok {
			rs.Note, rs.Cells = ob.Note, ob.Cells
			rs.Gaps = appendUnique(rs.Gaps, ob.Gaps...)
			rs.Result = scenarioResult(o.Run, s, ob, problem)
		}
		res.Scenarios = append(res.Scenarios, rs)
	}

	switch o.Mode {
	case "planning":
		res.Result, res.ReasonCode = "INCONCLUSIVE", "no_runner"
		res.Reason = o.Reason
		if len(obs) > 0 {
			problem("planning mode ignores %d observation file(s)", len(obs))
		}
	case "na":
		res.Result, res.ReasonCode, res.Reason = "NA", "no_phone_runtime_change", o.Reason
		if o.Reason == "" {
			return res, errors.New("--mode na needs --reason")
		}
	case "", "device":
		res.Result, res.ReasonCode = deviceResult(res.Scenarios, l)
		res.Reason = o.Reason
	default:
		return res, fmt.Errorf("--mode %q: want device, planning or na", o.Mode)
	}

	res.Gates = gates(res)
	if err := writeJSON(filepath.Join(o.Run, "result.json"), res); err != nil {
		return res, err
	}
	forbidden := h.privateLiterals(o.Run, l)
	summary, redacted := publicSafe(prSummary(res, plan), forbidden)
	if redacted > 0 {
		res.Problems = append(res.Problems, fmt.Sprintf("pr-summary.md: redacted %d private token(s); reword the plan text that carried them", redacted))
		writeJSON(filepath.Join(o.Run, "result.json"), res)
	}
	if err := os.WriteFile(filepath.Join(o.Run, "pr-summary.md"), []byte(summary), 0o600); err != nil {
		return res, err
	}
	if err := os.WriteFile(filepath.Join(o.Run, "report.md"), []byte(privateReport(o.Run, res)), 0o600); err != nil {
		return res, err
	}
	return res, writeManifest(o.Run)
}

func appendUnique(a []string, b ...string) []string {
	for _, x := range b {
		if !slices.Contains(a, x) {
			a = append(a, x)
		}
	}
	return a
}

// scenarioResult trusts cells over the runner's headline: a planned cell with no passing observation keeps the
// scenario from PASS, and evidence must exist inside the run.
func scenarioResult(run string, s PlanScenario, ob Observation, problem func(string, ...any)) string {
	if !slices.Contains(deviceResults, ob.Result) {
		problem("%s: result %q is not one of %v", s.ID, ob.Result, deviceResults)
		return "INCONCLUSIVE"
	}
	seen := map[string]string{}
	missingEvidence := false
	for _, c := range ob.Cells {
		if !slices.Contains(deviceResults, c.Result) {
			problem("%s/%s: result %q is not one of %v", s.ID, c.ID, c.Result, deviceResults)
			c.Result = "INCONCLUSIVE"
		}
		if c.Result == "PASS" || c.Result == "FAIL" {
			if len(c.Evidence) == 0 {
				problem("%s/%s: %s with no evidence", s.ID, c.ID, c.Result)
				missingEvidence = true
			}
			for _, e := range c.Evidence {
				if filepath.IsAbs(e) || strings.Contains(e, "..") {
					problem("%s/%s: evidence %q must be a path inside the run directory", s.ID, c.ID, e)
					missingEvidence = true
				} else if _, err := os.Stat(filepath.Join(run, e)); err != nil {
					problem("%s/%s: evidence %q does not exist", s.ID, c.ID, e)
					missingEvidence = true
				}
			}
		}
		seen[c.ID] = c.Result
	}
	if ob.Result == "FAIL" || slices.Contains(mapsValues(seen), "FAIL") {
		if missingEvidence {
			return "INCONCLUSIVE"
		}
		return "FAIL"
	}
	if missingEvidence {
		return "INCONCLUSIVE"
	}
	if len(s.Cells) == 0 {
		if ob.Result == "PASS" && len(ob.Cells) == 0 {
			problem("%s: PASS with no observed cell or evidence", s.ID)
			return "INCONCLUSIVE"
		}
		return ob.Result
	}
	for _, c := range s.Cells {
		if seen[c.ID] != "PASS" {
			if ob.Result == "PASS" {
				problem("%s: reported PASS but planned cell %s is %q", s.ID, c.ID, seen[c.ID])
			}
			return "INCONCLUSIVE"
		}
	}
	return "PASS"
}

func mapsValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func deviceResult(ss []ResultScenario, l *Ledger) (string, string) {
	required, fail, missing := 0, false, false
	for _, s := range ss {
		if !s.Required {
			continue
		}
		required++
		switch s.Result {
		case "FAIL":
			fail = true
		case "PASS":
		default:
			missing = true
		}
	}
	switch {
	case fail:
		return "FAIL", "observed_defect"
	case len(l.Failures) > 0 && (missing || required == 0):
		return "INCONCLUSIVE", "environment"
	case missing || required == 0:
		return "INCONCLUSIVE", "missing_observation"
	}
	return "PASS", "observed"
}

func cleanupState(run string, l *Ledger) ResultCleanup {
	if len(l.Resources) == 0 {
		return ResultCleanup{Status: "nothing-owned"}
	}
	var c CleanupRecord
	if err := readJSON(filepath.Join(run, "cleanup.json"), &c); err != nil {
		return ResultCleanup{Status: "not-run", Failed: []string{"down has not run: the run still owns device resources"}}
	}
	var left []string
	for _, r := range l.Resources {
		if r.State != stateRemoved {
			left = append(left, fmt.Sprintf("%d %s (%s) %s", r.ID, r.Kind, r.State, r.Error))
		}
	}
	if len(left) > 0 {
		return ResultCleanup{Status: "failed", Failed: left}
	}
	return ResultCleanup{Status: "ok"}
}

func gates(r Result) Gates {
	g := Gates{DeviceQA: r.Result, OutsideQA: "PASS", Manual: "CONFIRMED", Cleanup: r.Cleanup.Status}
	for _, p := range r.OutsideQA {
		switch {
		case p.State == "FAIL":
			g.OutsideQA = "FAIL"
		case p.State == "PENDING" && g.OutsideQA != "FAIL":
			g.OutsideQA = "PENDING"
		}
	}
	for _, m := range r.ManualCriteria {
		switch {
		case m.State == "REJECTED":
			g.Manual = "REJECTED"
		case m.State == "PENDING" && g.Manual != "REJECTED":
			g.Manual = "PENDING"
		}
	}
	g.StageClean = (r.Result == "PASS" || r.Result == "NA") &&
		(g.Cleanup == "ok" || g.Cleanup == "nothing-owned") && !r.Target.HeadMismatch && g.OutsideQA != "FAIL"
	return g
}

func (h *Helper) privateLiterals(run string, l *Ledger) []string {
	var out []string
	add := func(s string) {
		if len(s) >= 3 && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	if hn, err := os.Hostname(); err == nil {
		add(hn)
	}
	if u, err := user.Current(); err == nil {
		add(u.Username)
		add(u.HomeDir)
	}
	add(h.Getenv("HOME"))
	add(h.Getenv("USER"))
	add(run)
	add(h.Repo)
	if l != nil {
		add(l.Serial)
	}
	return out
}

var privatePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}(?::\d+)?\b`),
	regexp.MustCompile(`(?i)\b[0-9a-f]{1,4}(?::[0-9a-f]{0,4}){2,7}\b`),
	regexp.MustCompile(`(?i)\b(?:[0-9a-f]{2}[:-]){5}[0-9a-f]{2}\b`),
	regexp.MustCompile(`\bemulator-\d+\b`),
	regexp.MustCompile(`(?i)\b[a-z0-9-]+\.ts\.net\b`),
	regexp.MustCompile(`(?:^|[\s(\x60"'])(?:~|/(?:home|Users|root|tmp|run|var|data))/[^\s)\x60"']*`),
}

// publicSafe removes every private literal and every address, MAC, serial or local path from text meant for a public
// PR, and counts what it removed so the caller can fix the source rather than ship a redaction silently.
func publicSafe(text string, literals []string) (string, int) {
	n := 0
	slices.SortFunc(literals, func(a, b string) int { return len(b) - len(a) })
	for _, lit := range literals {
		if c := strings.Count(text, lit); c > 0 {
			n += c
			text = strings.ReplaceAll(text, lit, "[redacted]")
		}
	}
	for _, re := range privatePatterns {
		text = re.ReplaceAllStringFunc(text, func(m string) string {
			n++
			lead := ""
			if m != "" && strings.ContainsRune(" \t\n(`\"'", rune(m[0])) {
				lead = m[:1]
			}
			return lead + "[redacted]"
		})
	}
	return text, n
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "|", "/")), " ")
}

func prSummary(r Result, plan Plan) string {
	var b strings.Builder
	b.WriteString("## QA\n\n")
	switch r.Result {
	case "NA":
		fmt.Fprintf(&b, "Device QA: **NA** (no phone runtime change): %s\n", oneLine(r.Reason))
	case "INCONCLUSIVE":
		if r.ReasonCode == "no_runner" {
			fmt.Fprintf(&b, "Device QA: **INCONCLUSIVE, planning only**: no device run happened for `%s`. Nothing below was observed on a device.\n", shortSHA(r.Target.TestedSHA))
		} else {
			fmt.Fprintf(&b, "Device QA: **INCONCLUSIVE** (%s) on `%s`.\n", r.ReasonCode, shortSHA(r.Target.TestedSHA))
		}
	default:
		fmt.Fprintf(&b, "Device QA: **%s** on `%s`", r.Result, shortSHA(r.Target.TestedSHA))
		if r.APK != nil {
			fmt.Fprintf(&b, ", R8-minified release APK sha256 `%s`", shortSHA(r.APK.SHA256))
		}
		b.WriteString(".\n")
	}
	if r.Target.HeadMismatch {
		fmt.Fprintf(&b, "\nThe branch moved to `%s` after this run; these results are for `%s` only.\n", shortSHA(r.Target.HeadAtFinish), shortSHA(r.Target.TestedSHA))
	}
	fmt.Fprintf(&b, "\nGates: device QA %s; checks outside QA %s; manual confirmations %s; cleanup %s.\n",
		r.Gates.DeviceQA, r.Gates.OutsideQA, r.Gates.Manual, r.Cleanup.Status)

	if len(r.Scenarios) > 0 {
		b.WriteString("\n| Scenario | Required | Result |\n| --- | --- | --- |\n")
		for _, s := range r.Scenarios {
			res := s.Result
			if r.ReasonCode == "no_runner" {
				res = "not exercised"
			}
			req := "no"
			if s.Required {
				req = "yes"
			}
			gaps := ""
			if len(s.Gaps) > 0 && res != "PASS" {
				gaps = " (" + strings.Join(s.Gaps, ", ") + ")"
			}
			fmt.Fprintf(&b, "| %s %s | %s | %s%s |\n", s.ID, oneLine(s.Title), req, res, gaps)
		}
	}
	if len(r.OutsideQA) > 0 {
		b.WriteString("\nProven outside device QA:\n\n| Criterion | Proof | State |\n| --- | --- | --- |\n")
		for _, p := range r.OutsideQA {
			fmt.Fprintf(&b, "| %s %s | %s | %s |\n", p.ID, oneLine(p.Criterion), oneLine(p.Proof), p.State)
		}
	}
	if len(r.ManualCriteria) > 0 {
		b.WriteString("\nManual confirmations:\n\n")
		for _, m := range r.ManualCriteria {
			fmt.Fprintf(&b, "- %s %s: **%s**\n", m.ID, oneLine(m.Criterion), m.State)
		}
	}
	if len(r.CapabilityGaps) > 0 {
		b.WriteString("\nCapability gaps:\n\n")
		for _, g := range r.CapabilityGaps {
			fmt.Fprintf(&b, "- %s %s%s\n", g.ID, oneLine(firstNonEmpty(g.Summary, g.Consequence)), blocks(g.Blocks))
		}
	}
	if len(r.SpecDisagreements) > 0 {
		b.WriteString("\nSpec disagreements:\n\n")
		for _, d := range r.SpecDisagreements {
			fmt.Fprintf(&b, "- %s spec: %s; implementation: %s\n", d.ID, oneLine(d.Spec), oneLine(d.Implementation))
		}
	}
	if len(r.SpecGaps) > 0 {
		b.WriteString("\nUnspecified behavior (no device oracle):\n\n")
		for _, g := range r.SpecGaps {
			fmt.Fprintf(&b, "- %s %s\n", g.ID, oneLine(firstNonEmpty(g.Summary, g.Coverage)))
		}
	}
	return b.String()
}

func blocks(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return " (blocks " + strings.Join(ids, ", ") + ")"
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

func privateReport(run string, r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# QA report: %s %s (%s)\n\n", r.Result, r.ReasonCode, r.Target.Branch)
	if r.Reason != "" {
		fmt.Fprintf(&b, "%s\n\n", r.Reason)
	}
	fmt.Fprintf(&b, "Private: this file links local paths. Paste only pr-summary.md publicly.\n\n")
	fmt.Fprintf(&b, "- Tested SHA: %s (merge base %s), head at finish %s, mismatch %v\n", r.Target.TestedSHA, r.Target.MergeBase, r.Target.HeadAtFinish, r.Target.HeadMismatch)
	if r.APK != nil {
		fmt.Fprintf(&b, "- APK: %s, sha256 %s, signer %s, debuggable %v, R8 mapping %v\n", r.APK.Path, r.APK.SHA256, r.APK.SignerSHA256, r.APK.Debuggable, r.APK.R8Mapping)
	}
	fmt.Fprintf(&b, "- Gates: %+v\n- Cleanup: %s %v\n", r.Gates, r.Cleanup.Status, r.Cleanup.Failed)
	for _, name := range []string{"plan.md", "plan.json", "result.json", "pr-summary.md", "evidence-manifest.json", "ledger.json", "cleanup.json", "commands.log", "up.json"} {
		if _, err := os.Stat(filepath.Join(run, name)); err == nil {
			fmt.Fprintf(&b, "- [%s](%s)\n", name, filepath.Join(run, name))
		}
	}
	if len(r.Problems) > 0 {
		b.WriteString("\n## Problems\n\n")
		for _, p := range r.Problems {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}
	b.WriteString("\n## Scenarios\n\n| ID | Required | Result | Cells | Note |\n| --- | --- | --- | --- | --- |\n")
	for _, s := range r.Scenarios {
		var cells []string
		for _, c := range s.Cells {
			cells = append(cells, fmt.Sprintf("%s=%s (%s, font %s, %s) %s", c.ID, c.Result, c.Theme, c.FontMultiplier, c.Appearance, strings.Join(c.Evidence, " ")))
		}
		fmt.Fprintf(&b, "| %s %s | %v | %s | %s | %s |\n", s.ID, oneLine(s.Title), s.Required, s.Result, oneLine(strings.Join(cells, "; ")), oneLine(s.Note))
	}
	return b.String()
}

// ManifestEntry lists one file of evidence. Public is always false: only a person or the coordinator, after looking
// at the file, may decide that it can leave this machine.
type ManifestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Media  bool   `json:"media"`
	Public bool   `json:"public"`
}

func writeManifest(run string) error {
	var entries []ManifestEntry
	err := filepath.WalkDir(run, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(run, p)
		if d.IsDir() {
			if rel == "signing" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "evidence-manifest.json" || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		sum, n, err := fileDigest(p)
		if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(p))
		entries = append(entries, ManifestEntry{Path: rel, SHA256: sum, Bytes: n, Media: slices.Contains([]string{".png", ".jpg", ".jpeg", ".mp4", ".webm", ".gif"}, ext)})
		return nil
	})
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(run, "evidence-manifest.json"), entries)
}
