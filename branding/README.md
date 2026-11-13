# Anshin branding

The selected direction combines Interval's logo with Level / periwinkle's app layouts and palette. The Owner chose this combination after four independent explorations. Anshin remains the product name, with a lowercase `anshin` wordmark and the tagline "Your systems. At peace."

The [combined board](board.html) shows the selected identity, Dashboard, Machine detail, semantic palette, state examples and launcher construction. Every resource value on the board is illustrative. The board is a design reference for Kotlin/Compose; it is not an implemented app.

## Logo

The paired open frames identify Anshin. They use Interval's geometry and lettering, recolored with the selected palette. The logo never changes with a Machine's Verdict or connection state.

| Source | Use |
| --- | --- |
| [symbol.svg](symbol.svg) | Standalone periwinkle symbol, 24-unit grid |
| [wordmark.svg](wordmark.svg) | Bone-colored lettering without the symbol |
| [lockup.svg](lockup.svg) | Periwinkle symbol beside the lowercase lettering |
| [monochrome.svg](monochrome.svg) | Single black ink for masks or recoloring |
| [icon-foreground.svg](icon-foreground.svg) | Transparent 108-unit Android launcher foreground |
| [adaptive-icon.svg](adaptive-icon.svg) | Construction guide and mask previews, not the launcher foreground |

Keep at least one stroke width clear around the standalone symbol. Scale it uniformly, preserve the opposing openings, and use a single ink. Use the lockup for introductions where the abstract symbol alone would be unfamiliar. The wordmark's custom lettering belongs in the identity; use readable system type for the interface.

The launcher foreground places the 24-unit mark at twice its source scale, centered on a 108-unit canvas. The painted bounds fit inside the central 66-unit safe circle. The construction guide is never part of the installed icon. No trademark clearance is implied.

## Palette and themes

[palette.json](palette.json) defines the selected dark palette, **Ink & bone**. It is the default app theme and the fixed launcher palette. This choice replaces the original brief's Kanagawa default. Keep four selectable dark themes: Ink & bone, Tokyo Night, Catppuccin Mocha and Matte Black. The alternate themes' values, upstream sources, licenses and substitutions are in [themes/README.md](themes/README.md); the exploration color chips are not authoritative.

| Token | Hex | Meaning |
| --- | --- | --- |
| background | `#141519` | Screen background |
| surface | `#1B1C22` | Machine panels and tracks |
| border | `#2A2C35` | Quiet rules and outlines |
| text | `#E8E4DA` | Primary text |
| muted | `#9A978F` | Labels, units and supporting copy |
| accent | `#A5A8F5` | Brand, navigation, selection and Live marker |
| cpu | `#7DB7D6` | CPU series |
| memory | `#C79BC9` | Memory series |
| storage | `#74BFB0` | Filesystem measurements |
| network | `#C4B998` | Network measurements |
| ok | `#93C47D` | Current Agent Verdict `ok` |
| warn | `#E8B04C` | Current Agent Verdict `warn` and its Reasons |
| crit | `#EC6A5E` | Current Agent Verdict `crit` and its Reasons |
| unavailable | `#7F8396` | Absent or historical values and hatching, always labelled |

Periwinkle never means `ok`. Live describes fresh Pulses, independently of the Verdict. Pair meaning with words and shape; color alone is insufficient. Alternate themes use the same token roles, geometry and hierarchy. Android themed icons use the monochrome layer.

## App layout and state

Use Level / periwinkle's compact resource views. Both Machines appear together on the Dashboard, below the worst current Verdict and its Reason. Each Machine shows freshness, CPU and memory sparklines, root filesystem usage, and available supporting measurements. Detail uses ruled resource sections with per-core CPU, load, memory, filesystems and network. Native scrolling continues to battery and Machine information.

Level's vial remains a secondary information glyph beside a Verdict or connection label. It is not the logo, a control, or a toggle. Always pair it with literal words. Centered, offset and filled variants accompany `ok`, `warn` and `crit`; non-Live variants use the unavailable color. Unknown Verdict strings get a neutral glyph and their text, never a guessed severity.

An incomplete view must name the Machine whose current Verdict is unavailable. Any remaining Verdict is scoped to the Live Machine. Stale values carry their age and quote a last Verdict only as historical text. Unreachable does not guess a cause. Forbidden and Incompatible withhold a current Verdict. Missing measurements stay unavailable, and a desktop says "No battery". Charts break at each Gap, with hatching at its true timestamp width. They never join across it.

Show threshold ticks only where the Agent's documented Verdict rules define them. The Agent computes the Verdict; the phone renders it. CPU usage itself has no Verdict threshold. Label all measurement units. Resource colors stay consistent across Machines and screens.

Use system sans for UI copy and JetBrains Mono with tabular numerals for numbers, units, paths and Verdict words. The app bundles JetBrains Mono NL 2.304 Regular with its OFL license in `android/app/src/main/assets/fonts/`. Start with 8 dp Machine-panel corners, 1 dp borders and zero elevation. Native tap targets are at least 48 dp. Larger font settings take precedence over fitting both Machines above the fold. Provide text summaries for charts and avoid announcing every arriving Sample to TalkBack.

The API remains read-only. Phone traffic stops when the app is not visible. Add/Edit Machine changes the phone's configuration; it does not operate the Linux Machine. There are no remote actions or background notifications in this design.

## Android icon resources

[android/res](android/res) contains the resource pack the app module copies:

- `drawable/ic_anshin.xml` for the in-app mark.
- `drawable/ic_launcher_foreground.xml` and `drawable/ic_launcher_monochrome.xml` on the 108-unit launcher grid.
- `mipmap-anydpi-v26/ic_launcher.xml` for adaptive icons, with the themed monochrome layer added in the v33 override.
- `values/anshin_colors.xml` for the fixed launcher colors.

The app keeps copies in `android/app/src/main/res`. Its `minSdk` is 26, so it has a single `mipmap-anydpi/ic_launcher.xml`, the v33 variant with the monochrome layer, which earlier launchers ignore. Tint the in-app mark from the active theme's semantic accent; keep the launcher colors fixed. The resources compile in the app; they haven't been installed on a device yet. Native font scaling, TalkBack, touch behavior and launcher rendering remain to be validated in the app.

## Sources and regeneration

The editable sources are [symbol.svg](symbol.svg), [wordmark.svg](wordmark.svg) and [palette.json](palette.json). Run `python3 branding/build.py` from the repository root to generate the lockup, monochrome and launcher assets, Android resources and combined board. The board reuses the original Level / periwinkle layout and chart illustrations from `explorations/03-fable/board.html`.

`make check-brand`, part of `make check` and CI, reruns the generator on a copy of this directory and fails, naming each path, when a generated file differs from what `build.py` produces or is missing. To fix a failure, run `python3 branding/build.py` and commit its outputs; never hand-edit a generated file.

The [original comparison](explorations/comparison.html) and all four exploration directories are retained as provenance. They contain superseded proposals, including the Level logo and the earlier default-theme assumptions. This document and the root branding assets define the selected direction.
