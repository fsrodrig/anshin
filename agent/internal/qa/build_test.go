package qa

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A theme missing from ThemeKeys gets no seed. Android's ThemeTokensTest checks the app against the same palettes.
func TestThemeKeysMatchBrandPalettes(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "..", "branding", "themes"))
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{"ink-and-bone"}
	for _, entry := range entries {
		if key, ok := strings.CutSuffix(entry.Name(), ".json"); ok && !entry.IsDir() {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	if !slices.Equal(keys, slices.Sorted(slices.Values(ThemeKeys))) {
		t.Fatalf("branding palette keys %v, ThemeKeys %v", keys, ThemeKeys)
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
