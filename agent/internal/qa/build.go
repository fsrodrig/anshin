package qa

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// AppID is the package the QA build installs. A different one in the built APK means the wrong app was built.
const AppID = "io.github.fsrodrig.anshin"

// ThemeKeys are the stored keys of the four themes, as Themes.kt names them. QaThemeSeedTest writes one seed per key.
var ThemeKeys = []string{"ink-and-bone", "tokyo-night", "catppuccin-mocha", "matte-black"}

// SDK is where the Android tools the helper needs live. The signing tools are not on PATH in a stock SDK install,
// so they are found in the newest build-tools directory instead.
type SDK struct {
	Root       string `json:"root"`
	BuildTools string `json:"buildTools"`
	ADB        string `json:"adb"`
	Zipalign   string `json:"zipalign"`
	Apksigner  string `json:"apksigner"`
	AAPT2      string `json:"aapt2"`
}

func findSDK(repo string, getenv func(string) string) (SDK, error) {
	root := getenv("ANDROID_HOME")
	if root == "" {
		root = getenv("ANDROID_SDK_ROOT")
	}
	if root == "" {
		root = sdkDirFromLocalProperties(filepath.Join(repo, "android", "local.properties"))
	}
	if root == "" {
		return SDK{}, errors.New("no Android SDK: set ANDROID_HOME or sdk.dir in android/local.properties")
	}
	s := SDK{Root: root, ADB: filepath.Join(root, "platform-tools", "adb")}
	entries, err := os.ReadDir(filepath.Join(root, "build-tools"))
	if err != nil {
		return s, fmt.Errorf("no build-tools in the Android SDK: %w", err)
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			versions = append(versions, e.Name())
		}
	}
	slices.SortFunc(versions, compareVersions)
	for i := len(versions) - 1; i >= 0; i-- {
		dir := filepath.Join(root, "build-tools", versions[i])
		if isExecutable(filepath.Join(dir, "zipalign")) && isExecutable(filepath.Join(dir, "apksigner")) && isExecutable(filepath.Join(dir, "aapt2")) {
			s.BuildTools = dir
			s.Zipalign, s.Apksigner, s.AAPT2 = filepath.Join(dir, "zipalign"), filepath.Join(dir, "apksigner"), filepath.Join(dir, "aapt2")
			return s, nil
		}
	}
	return s, errors.New("no build-tools version has zipalign, apksigner and aapt2")
}

func sdkDirFromLocalProperties(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "sdk.dir="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x - y
		}
	}
	return strings.Compare(a, b)
}

func isExecutable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

// APKRecord is what a report cites about the build it tested.
type APKRecord struct {
	TargetSHA         string `json:"targetSha"`
	Path              string `json:"path"`
	SHA256            string `json:"sha256"`
	SizeBytes         int64  `json:"sizeBytes"`
	Package           string `json:"package"`
	VersionCode       string `json:"versionCode"`
	VersionName       string `json:"versionName"`
	Debuggable        bool   `json:"debuggable"`
	R8Mapping         bool   `json:"r8Mapping"`
	SignerSHA256      string `json:"signerSha256"`
	SignatureVerified bool   `json:"signatureVerified"`
	BuildTools        string `json:"buildTools"`
}

var (
	badgingPackage = regexp.MustCompile(`^package: name='([^']*)' versionCode='([^']*)' versionName='([^']*)'`)
	signerDigest   = regexp.MustCompile(`(?m)^(?:Signer #1|V[0-9.]+ Signer:) certificate SHA-256 digest: ([0-9a-f]{64})$`)
)

// buildRelease builds the R8-minified release APK from the clean checkout, then aligns, signs and verifies a copy in
// the run directory with a throwaway key made for this run. Nothing about release signing in Gradle changes, and no
// release secret is involved.
func (h *Helper) buildRelease(ctx context.Context, run string, info RunInfo, sdk SDK) (APKRecord, error) {
	rec := APKRecord{TargetSHA: info.TargetSHA, BuildTools: filepath.Base(sdk.BuildTools)}
	android := filepath.Join(h.Repo, "android")
	gradle := Cmd{Name: filepath.Join(android, "gradlew"), Args: []string{"--warning-mode=fail", "assembleRelease"}, Dir: android}
	if _, errb, err := h.run(ctx, gradle); err != nil {
		return rec, cmdError(gradle, errb, err)
	}
	unsigned := filepath.Join(android, "app", "build", "outputs", "apk", "release", "app-release-unsigned.apk")
	mapping := filepath.Join(android, "app", "build", "outputs", "mapping", "release", "mapping.txt")
	buildDir := filepath.Join(run, "build")
	signRoot := filepath.Join(run, "signing")
	if err := os.MkdirAll(signRoot, 0700); err != nil {
		return rec, err
	}
	signDir, err := os.MkdirTemp(signRoot, "attempt-")
	if err != nil {
		return rec, err
	}
	for _, d := range []string{buildDir, signDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return rec, err
		}
	}
	copied := filepath.Join(buildDir, "app-release-unsigned.apk")
	if err := copyFile(unsigned, copied); err != nil {
		return rec, fmt.Errorf("the release build left no unsigned APK: %w", err)
	}
	if _, err := os.Stat(mapping); err == nil {
		rec.R8Mapping = true
		copyFile(mapping, filepath.Join(buildDir, "mapping.txt"))
	}

	ks, pass := filepath.Join(signDir, "qa.p12"), filepath.Join(signDir, "qa.pass")
	if err := writeSecret(pass); err != nil {
		return rec, err
	}
	steps := []Cmd{
		{Name: "keytool", Args: []string{"-genkeypair", "-keystore", ks, "-storetype", "PKCS12", "-storepass:file", pass,
			"-alias", "qa", "-keyalg", "RSA", "-keysize", "2048", "-validity", "2", "-dname", "CN=anshin-qa throwaway"}},
		{Name: sdk.Zipalign, Args: []string{"-p", "-f", "4", copied, filepath.Join(buildDir, "app-release-aligned.apk")}},
		{Name: sdk.Apksigner, Args: []string{"sign", "--ks", ks, "--ks-pass", "file:" + pass, "--ks-key-alias", "qa",
			"--out", filepath.Join(buildDir, "app-release-qa.apk"), filepath.Join(buildDir, "app-release-aligned.apk")}},
	}
	for _, c := range steps {
		if _, errb, err := h.run(ctx, c); err != nil {
			return rec, cmdError(c, errb, err)
		}
	}
	rec.Path = filepath.Join(buildDir, "app-release-qa.apk")

	verify := Cmd{Name: sdk.Apksigner, Args: []string{"verify", "--print-certs", rec.Path}}
	out, errb, err := h.run(ctx, verify)
	if err != nil {
		return rec, cmdError(verify, errb, err)
	}
	m := signerDigest.FindSubmatch(out)
	if m == nil {
		return rec, errors.New("apksigner verified the APK but printed no signer digest")
	}
	rec.SignerSHA256, rec.SignatureVerified = string(m[1]), true

	badging := Cmd{Name: sdk.AAPT2, Args: []string{"dump", "badging", rec.Path}}
	out, errb, err = h.run(ctx, badging)
	if err != nil {
		return rec, cmdError(badging, errb, err)
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if p := badgingPackage.FindStringSubmatch(line); p != nil {
			rec.Package, rec.VersionCode, rec.VersionName = p[1], p[2], p[3]
		}
		if strings.HasPrefix(line, "application-debuggable") {
			rec.Debuggable = true
		}
	}
	if rec.Package != AppID {
		return rec, fmt.Errorf("the APK's package is %q, want %q", rec.Package, AppID)
	}
	if rec.Debuggable {
		return rec, errors.New("the release APK is debuggable; QA tests the shipped, non-debuggable build")
	}
	if !rec.R8Mapping {
		return rec, errors.New("the release build wrote no R8 mapping, so it may not be minified")
	}
	rec.SHA256, rec.SizeBytes, err = fileDigest(rec.Path)
	if err != nil {
		return rec, err
	}
	return rec, writeJSON(filepath.Join(buildDir, "apk.json"), rec)
}

// SeedRecord is one generated theme preference file.
type SeedRecord struct {
	Theme  string `json:"theme"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// generateSeeds has the real DataStore write each theme's preference file, through QaThemeSeedTest, which also reads
// every one back through ThemeSetting. A hand-written protobuf would be a second, untested encoder.
func (h *Helper) generateSeeds(ctx context.Context, run string) ([]SeedRecord, error) {
	dir := filepath.Join(run, "seeds")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	android := filepath.Join(h.Repo, "android")
	c := Cmd{Name: filepath.Join(android, "gradlew"), Dir: android, Args: []string{"--warning-mode=fail",
		":app:testDebugUnitTest", "--tests", "io.github.fsrodrig.anshin.QaThemeSeedTest", "--rerun", "-Panshin.qaSeedDir=" + dir}}
	if _, errb, err := h.run(ctx, c); err != nil {
		return nil, cmdError(c, errb, err)
	}
	var seeds []SeedRecord
	for _, key := range ThemeKeys {
		p := filepath.Join(dir, key+".preferences_pb")
		sum, _, err := fileDigest(p)
		if err != nil {
			return nil, fmt.Errorf("QaThemeSeedTest wrote no seed for %s: %w", key, err)
		}
		seeds = append(seeds, SeedRecord{Theme: key, Path: p, SHA256: sum})
	}
	return seeds, writeJSON(filepath.Join(dir, "seeds.json"), seeds)
}

func writeSecret(path string) error {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(hex.EncodeToString(b)+"\n"), 0o600)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func fileDigest(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
