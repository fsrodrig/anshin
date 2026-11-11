# Register

Anshin is a small instrument for checking two Machines. The identity uses two open brackets and one reference line. The brackets suggest a bounded Window; the line gives the two halves something in common. It does not indicate a connection or a good Verdict. I prefer this fixed mark to an animated Pulse, which could falsely suggest that fresh data is arriving.

The lowercase wordmark is drawn as vector paths. Its straight stems and rounded bowls echo the symbol without turning every letter into a pictogram. The symbol has three strokes, a 2.5-unit weight on a 24-unit grid, and no color-dependent detail. `monochrome.svg` supplies the same geometry in black. `adaptive-icon.svg` demonstrates a 108-unit Android layer with a 66-unit safe circle. The foreground's extreme corners stay inside that circle. Its dashed circle is a review guide and must be removed from production artwork. The generator keeps the untransformed mark geometry available for native vector conversion.

## Palette and hierarchy

The full hex token table is in concept.json and the board. Background and surface are warm charcoal; border divides rows without elevation. Text is warm white. Muted text is reserved for secondary explanations and timestamps, never the only indication of a failure.

Accent is pale blue for navigation and the identity. The Verdict colors are green for ok, amber for warn and coral for crit. They do not share the brand accent. CPU uses pale slate, memory lavender, storage sand and network muted teal. Unavailable uses neutral gray with explicit text. Chart colors identify quantities, not severity. A filesystem bar stays sand when its Verdict is warn; the labelled Verdict conveys the warning.

Kanagawa, Tokyo Night, Catppuccin Mocha and Matte Black remain selectable through the semantic token interface in the existing ui module. Each can substitute its own dark background, surface, text and chart colors. The three-stroke mark, wordmark, spacing and information order carry the identity across themes. Keep an interaction accent distinct from Verdict colors in each mapping. This original charcoal palette is an exploration, not an assertion of authentic upstream theme values. Upstream palettes and licenses require verification before implementation; no external sources were used here.

## Information and language

The Dashboard exposes both Machines together. The top line reports the worst current Verdict. Individual Machine rows keep connectivity separate from Verdict and place freshness immediately below the measurements. A Machine opens its detail screen with ordinary native navigation. The detail screen shows the Reason before charts.

Examples of intended voice:

- “Root filesystem is nearly full.”
- “Last Pulse 42s ago. Current Verdict unavailable.”
- “Cannot get a Pulse. Last received 3 min ago.”
- “The Agent refused this phone.”
- “This app cannot understand the Agent's Pulse version.”
- “Window restarted.”

When either Machine is Stale or Unreachable, the aggregate must say that the view is incomplete. For example, “1 Live · 1 Stale. Known Verdict: warn.” Never turn a historical ok into current reassurance. Retained measurements must carry their age; historical Verdicts, if exposed deeper in the UI, also need explicit historical labels. Forbidden and Incompatible provide no current Verdict. Unknown enum strings need the contract's unknown fallback rather than a fabricated label.

Missing measurements render as “Unavailable”; an absent battery renders as “No battery”. Unavailable inputs are excluded from the Agent's Verdict, not counted as healthy. A Gap leaves a blank interval at its actual timestamps. The board uses separate SVG paths on either side of the Gap. No line, smoothing or zero-valued baseline bridges it. A new Agent epoch replaces the Window and explains the reset; it does not preserve invented continuity.

All Machine aliases, chart shapes, values and ages are fictional and visibly labelled illustrative. They describe the proposed layout, not measurements of this repository or either real Machine. Root filesystem percentages align with the shown capacities. The CPU chart ends at approximately 18% on its declared 0–100% scale.

## Native implementation intent

This HTML is a presentation artifact only. Use the existing Kotlin/Compose stack and ui module. Rows use a 4dp spacing rhythm, 16dp screen gutters, modest 6dp control corners and no shadows. Device framing belongs only to the board. The native app uses system sans-serif for prose and the specified JetBrains Mono for numbers. The self-contained board uses local system fonts and ui-monospace instead of bundling fonts.

Use Material top app bars, minimum 48dp interaction targets, ordinary Android Back and scrollable content. Measurement tabs change the detail section; Settings contains theme, Machine configuration and app information. The Add Machine action configures the phone's MachineBook, not the Machine itself. It introduces no credentials or remote command capability.

Compose Canvas can draw the Window using timestamp coordinates and separate paths for null intervals. Provide a text summary for chart accessibility. Announce the Machine alias, connectivity, age and Verdict together; do not announce every new Sample automatically. Mark decorative logo paths as decorative when adjacent to the wordmark. Keep text labels beside color and support larger font scales by stacking the measurement columns. No continuous animation is required.

The existing Watcher module remains responsible for freshness, Window merging and foreground collection. UI components consume its state rather than duplicating that logic. No new seam or adapter is needed for this exploration. Phone traffic stops when the app is not visible. The Agent retains its own Window. There are no notifications, background phone monitoring, shell actions or Machine controls.

## Concerns and compromises

The mark is deliberately abstract and needs the wordmark during introduction. Brackets are familiar technical punctuation, so distinctiveness rests on the exact paired geometry and shared center line; this is an original drawing, not a trademark clearance claim.

The compact detail tabs and long translated labels need native layout testing. Font scaling may push part of the second Machine below the fold. Accessibility should take precedence over fitting both at every scale. Subtle chart colors need labelled axes and a nonvisual summary.

Artifacts were checked structurally, without browser or device rendering as requested. No visual fit, touch behavior, theme contrast certification or native accessibility test is claimed. The SVG and JSON are source assets; the HTML buttons and tabs are intentionally static.
