# Interval

Anshin is a small, private view of two Machines. This direction treats that view as a register: aligned values, ruled sections and short explanations. Warm ink avoids a blue-black control-room aesthetic. Lilac gives navigation an identifiable color without assigning it a Verdict.

## Mark and lettering

Two opposing open frames share a central interval. Each stays independent, as each Agent answers the phone directly. The symbol has two paths, square terminals and slightly softened corners. At 24 px its stroke is 2.5 px; there are no fragile interior details. Its meaning does not depend on color. The lowercase wordmark is original path lettering, with a compact monoline rhythm and no font dependency.

The adaptive construction uses a 108-unit canvas. The foreground's painted bounds fit inside the central 66-unit safe circle. The dashed circle is a construction guide, not part of the launcher asset. Circle and rounded-square previews sit beside the construction. Export the background and foreground separately for Android; the supplied SVG is a demonstration, not a ready-to-install Android resource. Monochrome uses the same geometry.

## Semantic colors

| Role | Hex | Use |
| --- | --- | --- |
| background | #191A1C | Screen canvas |
| surface | #222326 | Reason grouping and selected containers |
| border | #414246 | Quiet section rules; never the only control boundary |
| text | #EFECE5 | Primary text and current values |
| muted | #B3B0AC | Supporting text and units |
| accent | #BDB3E9 | Identity, selection and navigation |
| cpu | #91B8D5 | CPU series |
| memory | #C4A6C8 | Memory series |
| storage | #C7BA95 | Filesystem capacity bars |
| network | #8CBFBD | RX and TX measurements |
| ok | #9BC59A | Agent Verdict `ok` |
| warn | #EBC06A | Agent Verdict `warn` |
| crit | #ED9185 | Agent Verdict `crit` |
| unavailable | #AAA6A1 | Absent or historical values, always labelled |

Storage remains neutral even when nearly full. The separate Verdict and Reason explain the threshold, rather than turning every resource chart into a warning. Live has a neutral dot and a word; it means fresh Pulses, not an `ok` Verdict. State text and symbols carry meaning alongside color.

The four documented dark themes remain selectable. Kanagawa remains the default. Tokyo Night, Catppuccin Mocha and Matte Black each map the same semantic roles to their respective palettes. The paired frames, vector lettering, rules, spacing and hierarchy carry the identity when colors change. Keep accent distinct from all Verdict colors in each mapping. This board proposes an alternative ink palette; it does not require a fifth theme or claim to reproduce any upstream theme. Theme palettes and licenses would need review during implementation.

## Information and truthfulness

The Dashboard fits two Machines without a carousel. Its summary shows the worst current Verdict, followed by one compact section per Machine. The illustrative `warn` comes from filesystem usage at 92%, consistent with the documented 90% warning threshold. High CPU usage alone is not a Reason. Memory used means total minus available.

The detail screen begins with freshness and the Reason, then shows the Window and resource sections. The example chart uses separate paths around a Gap. Real timestamp positions determine its width; neither interpolation nor a shaded area bridges it. The right endpoint represents approximately 18% on the labelled percentage scale. An Agent restart replaces the Window and explains why earlier Samples are gone.

Stale values carry their age on the same line. Unreachable does not guess whether the Machine, Agent or tailnet caused it. Neither state displays a reassuring current Verdict. If either Machine lacks a current Verdict, the Dashboard summary must say the view is incomplete, identify that Machine, and scope any remaining Verdict to the Live Machine. An `ok` Verdict only covers available inputs. A missing CPU temperature stays unavailable and a desktop says No battery.

All numbers in the mock screens and state examples are illustrative. They are not captures or performance measurements. Machine A and Machine B are fictional aliases. No Machine address or personal identifier appears in the assets.

## Voice

- “Reason · Filesystem / is nearly full.”
- “Last Pulse 48 s ago. Current Verdict unavailable.”
- “Cannot get a Pulse.”
- “CPU temperature · Unavailable.”
- “Agent restarted. The previous Window is no longer available.”
- “The Agent refused this phone.”
- “This app cannot understand the Agent's Pulse version.”

Use direct explanations. Avoid “all systems operational,” which overstates what unavailable inputs can establish. Do not promise that the phone keeps watching after the app closes.

## Compose implementation notes

Keep the existing module interfaces. The UI consumes MachineView from Watcher and does not derive connectivity from resource values. Do not add a theme plugin seam; semantic token sets are enough.

Use Material 3 foundations, a small top app bar, LazyColumn sections and native navigation. A Machine section is one accessible tap target. Add opens Add/Edit Machine; the detail overflow contains local alias/edit/remove actions only. The compact Dashboard navigation represents Machines and Settings. None of these actions changes the Linux Machine.

Use system Roboto for UI text and bundled JetBrains Mono with tabular numerals for measurements, as required by design.md. HTML uses system-ui and ui-monospace to stay self-contained. The custom wordmark is a vector, not UI typography. Start from 16 dp screen gutters, 8 dp spacing increments, modest 6 dp grouping corners and zero elevation. The HTML phones are reduced presentation drawings, not literal dp measurements. Native interactive targets must be at least 48 dp even around small icons. At larger font scales, stack measurement rows and allow the Dashboard to scroll.

Compose Canvas draws straight segments between valid Samples and breaks paths at null runs. Expose a text summary and selected Sample values to accessibility services. Avoid animating fresh chart segments or flashing Live indicators. TalkBack should announce state changes without reading every arriving Sample. The app collects only while visible, and the API remains read-only.

## Concerns and compromises

The mark is abstract and could read as framing or exchange. Pair it with the wordmark during introduction; it has not undergone a trademark or similarity search. The wordmark's lettering is intentionally distinctive but should not become a full interface font.

The detail screen is the upper portion of a scrollable view; per-core CPU, load, swap, complete filesystem and interface lists, and static Machine information still need downstream layouts. The board includes no setup or Settings screens. Color is paired with words, but native contrast, TalkBack, font scaling and touch targets still need device validation. No browser or preview renderer was run. Static structure and SVG validity were checked; visual fit remains unverified.
