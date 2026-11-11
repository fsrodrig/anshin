# 04 · Level

**Both Machines, on the level.**

A design exploration for Anshin's branding and app. Nothing here is implementation, and nothing here changes `docs/design.md`, the glossary or an ADR. Every number shown in `board.html` is an illustrative mock value. Machine A and Machine B are fictional aliases.

## The idea

A spirit level is something you glance at, not something you operate. It tells you one thing, it is read-only, and when the bubble sits between the lines you stop thinking about it. "On the level" also means honest. Those are Anshin's two promises: a calm Verdict, and no claim it can't back.

So the mark is a level vial, and the same vial is the Verdict glyph in the app.

| What the phone knows | Vial | Word shown |
|---|---|---|
| Live, Verdict ok | bubble centred between the graduations, ok colour | ok |
| Live, Verdict warn | bubble drifted across a graduation, warn colour | warn, plus Reasons |
| Live, Verdict crit | bubble hard against the end, crit colour | crit, plus Reasons |
| Live, unknown Verdict level | hollow bubble, centred, muted | the Agent's string, verbatim |
| Stale | solid frame, no bubble | Stale, with age |
| Unreachable | dashed frame, no bubble | Unreachable |
| Forbidden | dashed frame, no bubble | Forbidden |
| Incompatible | dashed frame, no bubble | Incompatible |

A solid empty vial means the phone holds Samples but none recent enough to trust. A dashed one means it holds no Pulse it can show. Neither can be mistaken for a reassuring Verdict, because the reassurance is the bubble and the bubble is absent. The unknown-level row is described here but not drawn on the board.

The dashboard's top line follows the same rule. It renders the worst Verdict across Live Machines, and names any Machine that isn't Live beside it: "Machine B ok · Machine A Stale", never "both ok".

## Decisions

- **One shape, two jobs.** The brand mark is always single-ink and centred. In-app vials are smaller and two-tone (neutral frame, coloured bubble). That difference keeps the logo from looking like a status.
- **Graduations only where a rule exists.** Memory and filesystem gauges carry hairlines at the warn and crit thresholds from the Verdict table. CPU has no threshold in that table, so it gets a sparkline and no graduation.
- **Sparklines share a fixed 0–100 % scale** across Machines, so the two cards can be compared at a glance. A quiet Machine draws a low line; that is the point.
- **A Gap is a hatched band.** The line stops and restarts on a continuous time axis. Scrubbing inside the band reads "no Samples".
- **Unavailable is a dash, a word or a hatched track.** Never a 0 and never an empty gauge, which would read as "0 % used". A desktop says "no battery". A blocked mount says "unavailable for 40 s".
- **Stale drains colour.** Last Samples stay on screen in the muted tokens with their age. Unreachable shows no gauges at all.
- **Dense, flat, native.** Hairline borders, no elevation, 8 dp card corners, 1 dp gauge corners. Detail sections are separated by rules, not nested cards. Standard top app bar, back arrow, icon buttons. No shell, kill, restart or notification affordance appears anywhere, because the API is read-only and the MVP has no background work.
- **No tailnet address on the dashboard.** It isn't health information, and it keeps identifiers out of screenshots. It belongs on the Edit machine screen.

## Semantic palette (dark)

| Token | Hex | Role |
|---|---|---|
| background | `#131415` | graphite page |
| surface | `#1B1D1F` | cards and chips |
| border | `#2E3135` | hairlines, gauge tracks, grid lines |
| text | `#D6D9DC` | primary ink, cool grey |
| muted | `#8D939A` | labels, units, Stale values |
| accent | `#E8D9A6` | ivory: brand mark, focus, primary action. Never a Verdict |
| cpu | `#5D94DA` | CPU line and per-core bars |
| memory | `#935CB2` | memory gauge |
| storage | `#29A8AF` | filesystem gauges |
| network | `#B9597D` | rx solid, tx dashed, one hue |
| ok | `#66BA7F` | bubble only |
| warn | `#F7AC55` | bubble, crossed threshold, Reason marker |
| crit | `#EF604E` | bubble, crossed threshold, Reason marker |
| unavailable | `#6B7178` | hatch and dash for null measurements and Gaps |

Text always wears `text` or `muted`; a coloured mark beside it carries the meaning.

Colour checks I ran, with the `validate_palette.js` script from the bundled dataviz skill, dark mode, surface `#1B1D1F`:

- cpu, memory, storage, network in that order: all checks pass. Worst adjacent pair is 8.4 under simulated deuteranopia and 15.3 with normal vision (OKLab distance × 100).
- ok and warn: 5.5 under simulated protanopia, which fails the script's target. Accepted only because a Verdict is never colour alone.
- accent against warn: 11.6 with normal vision. Distinct, but close enough that ivory must stay out of the vial.

WCAG contrast against `surface`, computed with a small script of my own: text 11.93, accent 12.00, muted 5.45, memory 3.55 (the lowest series colour), unavailable 3.43. The `unavailable` token is for marks; the word "unavailable" is set in `muted`.

### Four themes

`docs/design.md` specifies four selectable dark themes. They coexist with this identity because the identity is geometry. Each theme fills the same 14 tokens and the in-app mark takes that theme's accent. Graphite and ivory is the brand palette for the launcher icon, README and release art. Whether it is also one of the four in-app slots is the owner's decision. The theme tiles on the board use approximate colours and are there only to show the mark under other palettes.

## Voice

Short, lowercase where the contract is lowercase, no exclamation, no blame.

- All Live and ok: "Both Machines ok."
- Worst Verdict: "warn" then "Machine B · / is 91 % full"
- Stale: "Machine A is Stale. Last Pulse 4 min ago."
- Unreachable: "Can't get a Pulse from Machine B. Anshin doesn't know why."
- VPN hint: "hint · VPN looks off"
- Forbidden: "Machine B's Agent refused this phone. Only the Owner's untagged devices may watch it."
- Incompatible: "Machine B's Agent sent a Pulse this app can't read. Update Anshin on the phone."
- Agent restart: "Agent restarted 3 min ago. The earlier Window is gone."
- Gap: "Gap · 6 min · no Samples"
- Unavailable: "no battery", "— unavailable", "unavailable for 40 s"
- Footer: "Read-only. Anshin looks only while this screen is open."

## Notes for Compose

- **Vial.** One `Canvas` composable taking the `MachineView` and the Verdict level. A rounded rect, two lines and a rounded rect; a dashed `PathEffect` for the no-Pulse states. Give it a `contentDescription` of the word, so TalkBack says "warn", not "image".
- **Bubble motion.** If the bubble animates between positions, keep it short and honour the system's reduced-motion setting. A static jump is fine.
- **Gauge.** A `Canvas` or two `Box`es, 6 dp tall, with an optional list of threshold fractions. The unavailable variant draws hatch lines instead of a fill.
- **Charts.** Compose `Canvas` as the design doc says. Break the `Path` at every null run and draw the hatched band from the run's timestamps. No interpolation, no curve smoothing across a Gap.
- **Tokens.** One immutable class of 14 colours per theme, provided through a `CompositionLocal`. Add a unit test that fails if any theme's accent equals or sits too near its ok, warn or crit.
- **Type.** JetBrains Mono with `fontFeatureSettings = "tnum"` for numbers; the platform sans for everything else.
- **Icons.** `monochrome.svg` is drawn on the 24 grid and converts directly to a vector drawable. The adaptive foreground is `symbol.svg`'s mark scaled to about 48 dp wide on the 108 dp canvas, which keeps it inside the 66 dp safe zone. The monochrome layer is the same path.
- **Touch targets.** The board draws 40 px icon buttons to fit; the app should use 48 dp.

## Concerns and compromises

- The mark is wide, so it is small inside a round launcher mask. A taller variant would be easier to see and would stop being a level.
- Some people will see a bandage or a pill before they see a level. I kept the graduations protruding past the capsule to push against that.
- Bubble position is a new convention. It is never the only signal, but it is one more thing to learn.
- The mark ignores the brief's "two connected points and a pulse line" suggestion. It says nothing literal about a tailnet or about two Machines.
- Ivory is a restrained accent. If the owner wants a more saturated brand colour, the available hues are already taken by the series and Verdict tokens, so something would have to give.
- The wordmark letters are hand-built from arcs and stems. The s in particular is two elliptical arcs and would benefit from a type designer's eye before release.
- I did not open `board.html` in a browser. I read the CSS for fit at 728 px, checked that the markup is balanced, and rasterised the SVG assets and chart components with `rsvg-convert`. The whole-page layout is unverified.
