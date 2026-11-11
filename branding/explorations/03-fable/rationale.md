# 03-fable · Level

*On the level.* One of four independent branding explorations. A design proposal only: nothing here changes `docs/design.md`, the glossary or the stack.

## The idea

A spirit level reads the thing it sits on and never adjusts it. That is Anshin's read-only promise in one object, and "on the level" also means honest, which is the other promise: null stays null, a Gap stays a Gap, Stale is never dressed up as current.

The mark is the vial of a level, with its bubble at rest between two marks.

## Decisions

- **One glyph for brand and state.** The vial is the launcher icon and also the Verdict indicator beside every Machine.

  | State | Vial | Colour |
  |---|---|---|
  | Connecting | small dot | slate |
  | Live, ok | bubble at centre | ok |
  | Live, warn | bubble on the right mark | warn |
  | Live, crit | right end filled | crit |
  | Stale | hollow bubble | slate |
  | Unreachable | empty | slate |
  | Forbidden | opaque centre | slate |
  | Incompatible | diamond instead of a bubble | slate |

  A Verdict level the phone doesn't know (they are open strings) gets the empty vial plus the raw word in the text colour.
- **Verdict colours only while Live.** Stale quotes the last Verdict in words ("Verdict was ok") with no colour. Unreachable shows no Verdict at all.
- **Ticks are limits, the dot is now.** Filesystem tracks draw the 90 % and 95 % limits from the Verdict table as ticks. A warn value visibly sits on the mark, the same picture as the warn vial. CPU has no ticks because it is not a Verdict input.
- **The Verdict word is the Agent's word.** `ok`, `warn`, `crit`, lowercase, in the number face. The phone shows what the Agent said.
- **Dashboard for two.** Two bordered Machine panels fill the screen under a top line carrying the worst Verdict and its Reason. CPU and memory get a sparkline of the Window; the disk gets a level track, because a 30-minute line of disk use is flat and says nothing.
- **Detail is ruled sections, not cards.** Denser, and it reads like an instrument sheet: Verdict and Reasons, CPU, Memory, Filesystems, then Network, Battery and Machine info further down.
- **No address on the Dashboard.** With two Machines the alias is enough, and it keeps tailnet names out of screenshots. The address belongs in Machine info on the detail screen.

## Palette roles ("ink & bone")

| Token | Hex | Use |
|---|---|---|
| background | `#141519` | screen |
| surface | `#1B1C22` | Machine panels, bar tracks |
| border | `#2A2C35` | 1 dp rules and outlines |
| text | `#E8E4DA` | primary text, the vial outline |
| muted | `#9A978F` | labels, axes, captions |
| accent | `#A5A8F5` | Live marker, selection, the brand bubble |
| cpu | `#7DB7D6` | CPU ink |
| memory | `#C79BC9` | memory ink |
| storage | `#74BFB0` | filesystem ink |
| network | `#C4B998` | network ink |
| ok | `#93C47D` | Verdict ok |
| warn | `#E8B04C` | Verdict warn |
| crit | `#EC6A5E` | Verdict crit |
| unavailable | `#7F8396` | null values, Stale numbers, hatching |

Three rules hold in every theme:

1. Green, amber and red belong to the Verdict and nothing else.
2. The accent is one cool hue that never means ok.
3. Measurement inks are soft and always sit beside their label; storage is aqua rather than sand so it can't be mistaken for warn.

WCAG contrast against `background`, computed from the hex values: text 14.4, muted 6.3, accent 8.3, ok 9.1, warn 9.3, crit 5.9, unavailable 4.9.

### Four themes

The identity carries no hue of its own, so each theme fills the same 14 tokens from its own upstream palette under the three rules. A starting map, to be checked against upstream before use:

| Theme | accent | ok / warn / crit |
|---|---|---|
| Kanagawa | crystalBlue | springGreen / carpYellow / peachRed |
| Tokyo Night | blue | green / yellow / red |
| Catppuccin Mocha | lavender | green / yellow / red |
| Matte Black | near-white | the theme's own muted green / amber / red |

The launcher icon stays ink, bone and periwinkle whatever theme is selected; Android's themed-icon mode uses the one-colour layer. The theme chips on the board are approximate stand-ins, not upstream values.

## Voice

Short, literal, and it never claims more than the phone knows.

- Both ok: "Both Machines ok."
- warn: "warn · Machine B. / is 91 % full."
- Mixed: "Machine A ok. Machine B is Unreachable." Never "all ok" when a Machine isn't Live.
- Stale: "No fresh Pulse for 3 min. Showing 14:21:07."
- Unreachable: "No Pulse from Machine B. Anshin can't tell why."
- VPN hint: "VPN looks off."
- Forbidden: "The Agent answered but refused this phone. It allows only the Owner's untagged devices."
- Incompatible: "The Agent's Pulse is a version this app can't read."
- Unavailable: "CPU temp — unavailable", "battery — none on this Machine".
- Gap: "Gap · 6 min".
- After an Agent restart: "The Agent restarted. The Window starts at 14:13."

## Compose notes

- **`VerdictVial(state)`**: one `Canvas`. `drawRoundRect` with a `Stroke`, two `drawLine`, one `drawCircle`, all from `unit = size.width / 48`. Give it a `contentDescription` such as "Verdict warn" or "Stale, 3 minutes".
- **Motion**: the bubble may settle with a short `animateFloatAsState` when a Machine becomes Live. Nothing else moves. Skip it when the system animator scale is 0.
- **Sparkline**: one `Path` per run of non-null Samples, `moveTo` after every null run, never a `lineTo` across one. Gap and Stale hatching is a clipped loop of `drawLine`. No chart library.
- **Tokens**: a `data class` with the 14 colours, four instances, provided through a `staticCompositionLocalOf`. No colour literal in a composable. This fits the Roborazzi "state × theme" matrix already in the design.
- **Shape**: 8 dp panels, 4 dp chips, 1 dp borders, zero tonal elevation.
- **Type**: `FontFamily.Default` for UI. JetBrains Mono with `fontFeatureSettings = "tnum"` for numbers, as `docs/design.md` says. What bundling it costs in APK size has not been measured here and has to be checked against the budget.
- **Assets**: `symbol.svg` and `wordmark.svg` are stroked paths, which `VectorDrawable` supports directly. The adaptive foreground is the symbol at 1.2× centred in the 108 dp layer (about 53 dp wide, inside the 66 dp safe zone), with `monochrome.svg` as the `<monochrome>` layer.

## Concerns and compromises

- The board has not been seen in a browser. The SVG files and every inline chart and glyph were rasterised with `rsvg-convert` and inspected; the HTML layout was checked by reading the CSS.
- A capsule with a dot can read as a toggle, which would wrongly imply control. The two marks are the mitigation. Worth a five-second test with someone who hasn't seen it.
- The hollow Stale bubble sits at the last known position. If that still reads as "ok, faded", drop the ring and let Stale share the empty vial with a different word.
- Bubble position is hard to see at 30 dp. It is the third signal after the word and the colour.
- "ink & bone" is not one of the four themes. Using it in the app would mean a fifth theme or replacing one, which is the Owner's call and a change to the design doc.
- The mark does not picture the tailnet or the Pulse, which the brief asked for.
- Additions that are not in `docs/design.md`: the "range" line under each sparkline (min and max over the Window, computed on the phone) and the wording of the mixed-state top line.
- Not designed here: the network rx/tx chart, battery section, Add/Edit machine, Settings, Connecting, and the top line with a non-Live Machine.
- The symbol is stroke-based. For a context that needs a single filled path, outline the strokes on export.
- All numbers on the board are illustrative. They are not measurements of any Machine or of Anshin.
