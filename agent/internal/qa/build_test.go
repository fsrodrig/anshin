package qa

import (
	"strings"
	"testing"
)

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
