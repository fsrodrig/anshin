package capture

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Rule names match scripts/identifier-scan.sh, which checks the same classes after the fact.
const (
	RuleSerial       = "serial number"
	RuleMAC          = "MAC address"
	RuleMACInterface = "MAC-named interface"
	RuleHostname     = "hostname"
	RuleDenylist     = "private denylist"
	RuleHome         = "home directory"
	RuleUsername     = "username"
	RuleTailnetName  = "tailnet name"
	RuleTailnetV4    = "tailnet IPv4 address"
	RuleTailnetV6    = "tailnet IPv6 address"
)

const (
	scrubbedSerial = "SCRUBBED"
	scrubbedWord   = "scrubbed"
	scrubbedUser   = "user"
	exampleTailnet = "example.ts.net"
	exampleV4      = "100.64.0.1"
	exampleV6      = "fd7a:115c:a1e0::1"
)

// placeholders must survive scrubbing unchanged, so scrubbing is idempotent and no term may hide inside one.
var placeholders = []string{scrubbedSerial, scrubbedWord, scrubbedUser, exampleTailnet, exampleV4, exampleV6,
	"x000000000000"}

const hex = `[0-9A-Fa-f]`

var (
	serialRe = regexp.MustCompile(`(?i)((?:serial_number|product_serial|board_serial|chassis_serial|product_uuid|id_serial(?:_short)?)["']?\s*[=:]\s*["']?)[A-Za-z0-9][^"'\s]*`)
	// A run of six or more two-digit octets with one separator throughout. The scan treats only six as a MAC, but
	// zeroing a longer run costs nothing and catches a MAC embedded in one.
	macRe          = regexp.MustCompile(`\b(?:` + hex + `{2}:){5,}` + hex + `{2}\b|\b(?:` + hex + `{2}-){5,}` + hex + `{2}\b`)
	hexDigit       = regexp.MustCompile(hex)
	macInterfaceRe = regexp.MustCompile(`(?i)\b(en|wl|ww)x[0-9a-f]{12}\b`)
	homeRe         = regexp.MustCompile(`(^|[^A-Za-z0-9_.~-])(/(?:var/)?home|/run/media|/media)/[A-Za-z0-9_.-]+`)
	tailnetNameRe  = regexp.MustCompile(`(?i)[a-z0-9-]+\.ts\.net\b`)
	tailnetV4Re    = regexp.MustCompile(`\b100\.(?:6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.[0-9]{1,3}\.[0-9]{1,3}(/[0-9]+)?`)
	tailnetV4Ok    = regexp.MustCompile(`^(?:100\.64\.0\.0/10|100\.64\.0\.[0-9]{1,3}(?:/[0-9]+)?|100\.127\.255\.255|100\.100\.100\.100)$`)
	tailnetV6Re    = regexp.MustCompile(`(?i)\bfd7a:115c:a1e0:[0-9a-f:]+(/[0-9]+)?`)
	tailnetV6Ok    = regexp.MustCompile(`(?i)^(?:fd7a:115c:a1e0::(?:[0-9a-f]{1,4})?(?:/[0-9]+)?|fd7a:115c:a1e0(?::ffff){5})$`)
)

type Terms struct {
	Hostnames []string
	Words     []string
	Users     []string
}

// minTermLen keeps a stray short entry from rewriting ordinary words all over a capture.
const minTermLen = 3

type Scrubber struct {
	replace map[string]string
	termRe  *regexp.Regexp
	words   map[string]bool
	userRe  *regexp.Regexp
	users   map[string]bool
	counts  map[string]map[string]int
}

func NewScrubber(name string, t Terms) (*Scrubber, error) {
	s := &Scrubber{replace: map[string]string{}, words: map[string]bool{}, counts: map[string]map[string]int{}}
	add := func(term, with string, word bool) error {
		lower := strings.ToLower(term)
		if len(lower) < minTermLen {
			return fmt.Errorf("a hostname or denylist entry is shorter than %d characters; scrubbing it would rewrite ordinary text", minTermLen)
		}
		if strings.Contains(strings.ToLower(name), lower) {
			return fmt.Errorf("-name %q contains a hostname or denylist entry; pick a name that doesn't", name)
		}
		for _, p := range placeholders {
			if strings.Contains(strings.ToLower(p), lower) {
				return fmt.Errorf("a hostname or denylist entry is part of the placeholder %q", p)
			}
		}
		if _, ok := s.replace[lower]; !ok {
			s.replace[lower] = with
			s.words[lower] = word
		}
		return nil
	}
	for _, h := range t.Hostnames {
		if err := add(h, name, false); err != nil {
			return nil, err
		}
	}
	for _, w := range t.Words {
		if err := add(w, scrubbedWord, true); err != nil {
			return nil, err
		}
	}
	if len(s.replace) > 0 {
		terms := slices.Collect(maps.Keys(s.replace))
		// Longest first, because Go's alternation takes the leftmost alternative that matches, not the longest.
		slices.SortFunc(terms, func(a, b string) int { return len(b) - len(a) })
		for i, term := range terms {
			terms[i] = regexp.QuoteMeta(term)
		}
		s.termRe = regexp.MustCompile(`(?i)(?:` + strings.Join(terms, "|") + `)`)
	}
	s.users = map[string]bool{}
	for _, u := range t.Users {
		if u != "" {
			s.users[strings.ToLower(u)] = true
		}
	}
	if len(s.users) > 0 {
		terms := slices.Collect(maps.Keys(s.users))
		slices.SortFunc(terms, func(a, b string) int { return len(b) - len(a) })
		for i := range terms {
			terms[i] = regexp.QuoteMeta(terms[i])
		}
		s.userRe = regexp.MustCompile(`(?i)(?:` + strings.Join(terms, "|") + `)`)
	}

	return s, nil
}

func (s *Scrubber) Scrub(file string, b []byte) []byte {
	return []byte(s.apply(file, string(b), true))
}

func (s *Scrubber) Quiet(text string) string { return s.apply("", text, false) }

func (s *Scrubber) Counts() map[string]map[string]int { return s.counts }

func (s *Scrubber) apply(file, text string, count bool) string {
	sub := func(rule string, re *regexp.Regexp, f func(m []string) string) {
		if re == nil {
			return
		}
		text = re.ReplaceAllStringFunc(text, func(match string) string {
			out := f(re.FindStringSubmatch(match))
			if out != match && count {
				if s.counts[rule] == nil {
					s.counts[rule] = map[string]int{}
				}
				s.counts[rule][file]++
			}
			return out
		})
	}
	sub(RuleSerial, serialRe, func(m []string) string { return m[1] + scrubbedSerial })
	sub(RuleMAC, macRe, func(m []string) string {
		return hexDigit.ReplaceAllString(m[0], "0")
	})
	sub(RuleMACInterface, macInterfaceRe, func(m []string) string { return m[1] + "x000000000000" })
	for _, rule := range []string{RuleHostname, RuleDenylist} {
		sub(rule, s.termRe, func(m []string) string {
			lower := strings.ToLower(m[0])
			if s.words[lower] != (rule == RuleDenylist) {
				return m[0]
			}
			return s.replace[lower]
		})
	}
	sub(RuleHome, homeRe, func(m []string) string { return m[1] + m[2] + "/" + scrubbedUser })
	if s.userRe != nil {
		var out strings.Builder
		last := 0
		asciiAlnum := func(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' }
		for _, loc := range s.userRe.FindAllStringIndex(text, -1) {
			a, z := loc[0], loc[1]
			octal := a >= 4 && text[a-4] == '\\'
			if octal {
				for _, b := range []byte(text[a-3 : a]) {
					octal = octal && b >= '0' && b <= '7'
				}
			}
			if a > 0 && asciiAlnum(text[a-1]) && !octal || z < len(text) && asciiAlnum(text[z]) {
				continue
			}
			out.WriteString(text[last:a])
			out.WriteString(scrubbedUser)
			last = z
			if count {
				if s.counts[RuleUsername] == nil {
					s.counts[RuleUsername] = map[string]int{}
				}
				s.counts[RuleUsername][file]++
			}
		}
		out.WriteString(text[last:])
		text = out.String()
	}
	sub(RuleTailnetName, tailnetNameRe, func(m []string) string {
		if strings.EqualFold(m[0], exampleTailnet) {
			return m[0]
		}
		return exampleTailnet
	})
	sub(RuleTailnetV4, tailnetV4Re, func(m []string) string {
		if tailnetV4Ok.MatchString(m[0]) {
			return m[0]
		}
		return exampleV4 + m[1]
	})
	sub(RuleTailnetV6, tailnetV6Re, func(m []string) string {
		if tailnetV6Ok.MatchString(m[0]) {
			return m[0]
		}
		return exampleV6 + m[1]
	})
	return text
}
