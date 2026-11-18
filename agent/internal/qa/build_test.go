package qa

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// A theme missing from ThemeKeys gets no seed, so the QA run could never plan a cell in it.
func TestThemeKeysAreThoseOfThemesKt(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "android", "app", "src", "main", "kotlin", "io", "github", "fsrodrig", "anshin", "Themes.kt"))
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, m := range regexp.MustCompile(`(?m)^    [A-Z]\w*\(\n        "([a-z-]+)",`).FindAllStringSubmatch(string(src), -1) {
		keys = append(keys, m[1])
	}
	if len(keys) == 0 || !slices.Equal(keys, ThemeKeys) {
		t.Fatalf("Themes.kt keys %v, ThemeKeys %v", keys, ThemeKeys)
	}
}

func TestSignerDigestSupportsSDKOutputFormats(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, prefix := range []string{"Signer #1", "V3.0 Signer:", "V3.1 Signer:"} {
		t.Run(prefix, func(t *testing.T) {
			match := signerDigest.FindStringSubmatch(prefix + " certificate SHA-256 digest: " + digest + "\n")
			if len(match) != 2 || match[1] != digest {
				t.Fatalf("signer digest not parsed: %v", match)
			}
		})
	}
}
