# Alternate themes

Tokyo Night, Catppuccin Mocha and Matte Black (dark), and Lupine and Flexoki Light (light), mapped onto the token roles of [Ink & bone](../palette.json). Each JSON file has the same shape as `palette.json`, and its `appearance` is `dark` or `light`; a light theme gets dark system bar icons. They are hand-written sources: the app never loads them, and `ThemeTokensTest` fails if the Kotlin tokens drift from them. The roles and the rules they follow are in [the branding guide](../README.md#palette-and-themes).

Every value comes from the upstream file named below, at the pinned commit. Where the upstream color for a role fails a check, the role takes the nearest color (by CIEDE2000) from the same upstream palette that passes, and the substitution is listed.

## The checks

`ThemeContrastTest` computes these for every theme, Ink & bone included:

- text, muted, accent, ok, warn, crit and unavailable reach 4.5:1 (WCAG AA) against both background and surface;
- cpu, memory, storage and network reach 3:1 against both;
- the accent stays apart from ok, warn and crit: CIEDE2000 ΔE00 ≥ 20 and a CIELCh hue difference ≥ 45° against each, and chroma C\*ab ≥ 30 so it reads as a color, not as text. Ink & bone's closest pair is accent and crit at ΔE00 37.6 and 100°;
- unavailable is a grey (C\*ab ≤ 15), so an absent value never looks like a series or a Verdict.

Two choices follow Ink & bone's hierarchy. Surface sits one quiet step from background (Ink & bone is 1.07:1), and border is a quiet rule against surface (Ink & bone is 1.22:1).

In a dark theme the panel is the lighter of the two grounds. In a light theme it may sit a step lighter or darker than its screen, whichever the upstream palette gives; the step stays quiet, about 1.04:1 to 1.12:1, and the border carries the panel's edge at about 1.22:1 against surface. Both light themes here put a slightly darker panel on a lighter screen: Lupine 1.05:1 with border 1.23:1, Flexoki Light 1.11:1 with border 1.25:1. Every text and series check is made on both grounds, so the darker one sets each light theme's limits.

## Tokyo Night

- Source: [tokyo-night/tokyo-night-vscode-theme](https://github.com/tokyo-night/tokyo-night-vscode-theme) (formerly `enkia/tokyo-night-vscode-theme`), the original theme, at commit [`7c0f11e`](https://github.com/tokyo-night/tokyo-night-vscode-theme/tree/7c0f11eaef322f293621ca7befe462214b7ea468) (version 1.1.2): the README's color palette table and `themes/tokyo-night-color-theme.json` (the Night variant).
- License: MIT, Copyright (c) 2018-present Enkia.

| Role | Hex | Upstream |
| --- | --- | --- |
| background | `#16161E` | `sideBar.background`, `panel.background` |
| surface | `#1A1B26` | `editor.background`; README "Editor Background (Night)" |
| border | `#363B54` | `textSeparator.foreground`, `terminal.ansiBlack` |
| text | `#C0CAF5` | README "Variables, Class names, Terminal White" |
| muted | `#9AA5CE` | `charts.foreground`; README "Markdown Text, HTML Text" |
| accent | `#7AA2F7` | README "Function names, CSS property names, Terminal Blue" |
| cpu | `#7DCFFF` | README "Terminal Cyan" |
| memory | `#BB9AF7` | README "Terminal Magenta" |
| storage | `#73DACA` | README "Terminal Green" |
| network | `#FF9E64` | README "Number and Boolean constants" |
| ok | `#9ECE6A` | README "Strings"; the palette's only true green, as its terminal green is teal |
| warn | `#E0AF68` | `editorWarning.foreground`; README "Terminal Yellow" |
| crit | `#F7768E` | substitution, below |
| unavailable | `#9699A8` | substitution, below |

Substitutions:

- crit: `editorError.foreground` `#DB4B4B` reaches only 4.17:1 on surface. It is replaced by README "Terminal Red" `#F7768E` (ΔE00 15.4), 6.46:1.
- unavailable: `disabledForeground` `#545C7E` reaches only 2.61:1 on surface. The nearest passing grey is `textPreformat.foreground` `#9699A8` (ΔE00 24.8), 6.04:1.

## Catppuccin Mocha

- Source: [catppuccin/palette](https://github.com/catppuccin/palette) `palette.json`, release [v1.8.0](https://github.com/catppuccin/palette/tree/07d02aa110ef9eb7e7427afca5c73ba9cf7f8ebd) (commit `07d02aa`), with roles from the [style guide](https://github.com/catppuccin/catppuccin/blob/d09787dd98ca6fba08af5ef2ae94a7e09f17daca/docs/style-guide.md) at commit `d09787d`. The accent is mauve, the default `catppuccin.accentColor` of [catppuccin/vscode](https://github.com/catppuccin/vscode).
- License: MIT, Copyright (c) 2021 Catppuccin.

| Role | Hex | Upstream |
| --- | --- | --- |
| background | `#181825` | Mantle ("Secondary Panes") |
| surface | `#1E1E2E` | Base ("Background Pane") |
| border | `#313244` | Surface 0 ("Surface Elements") |
| text | `#CDD6F4` | Text ("Body Copy") |
| muted | `#A6ADC8` | Subtext 0 ("Sub-Headlines, Labels") |
| accent | `#CBA6F7` | Mauve |
| cpu | `#74C7EC` | Sapphire |
| memory | `#F5C2E7` | Pink |
| storage | `#94E2D5` | Teal |
| network | `#FAB387` | Peach |
| ok | `#A6E3A1` | Green ("Success") |
| warn | `#F9E2AF` | Yellow ("Warnings") |
| crit | `#F38BA8` | Red ("Errors") |
| unavailable | `#9399B2` | substitution, below |

Panels are Base on a Mantle screen rather than Surface 0 on Base: Base on Mantle is the same quiet 1.07:1 step as Ink & bone, while Surface 0 panels (1.30:1) would push Overlay 1 and Overlay 2 below 4.5:1 and leave muted within ΔE00 6 of both text and unavailable.

Substitutions:

- unavailable: the style guide's "Subtle" Overlay 1 `#7F849C` reaches only 4.44:1 on surface. It is replaced by Overlay 2 `#9399B2` (ΔE00 7.1), 5.81:1.

## Matte Black

- Source: Omarchy's Matte Black theme, [`themes/matte-black/colors.toml`](https://github.com/omacom/omarchy/blob/v4.5.0/themes/matte-black/colors.toml) in [omacom/omarchy](https://github.com/omacom/omarchy) (formerly `basecamp/omarchy`) at tag v4.5.0 (commit `ac2690c`). That file has no green and no blue, so the remaining roles come from the theme author's own palette, which Omarchy's Matte Black loads for Neovim: [`lua/matteblack/colors.lua`](https://github.com/tahayvr/matteblack.nvim/blob/v1.0/lua/matteblack/colors.lua) in [tahayvr/matteblack.nvim](https://github.com/tahayvr/matteblack.nvim) at tag v1.0 (commit `a8a039f`).
- Licenses: Omarchy is MIT, Copyright (c) David Heinemeier Hansson; matteblack.nvim is MIT, Copyright (c) 2025 Taha Nejad.

| Role | Hex | Upstream |
| --- | --- | --- |
| background | `#121212` | Omarchy `background`; matteblack.nvim `bg` |
| surface | `#1E1E1E` | Omarchy `lighter_background` |
| border | `#333333` | Omarchy `muted`; matteblack.nvim `bg2` (window separators) |
| text | `#EAEAEA` | matteblack.nvim `fg` (Normal text); Omarchy `bright_cyan` |
| muted | `#BEBEBE` | Omarchy `foreground`; matteblack.nvim `fg2` |
| accent | `#3B82F6` | substitution, below |
| cpu | `#1EA7A0` | matteblack.nvim `cyan` |
| memory | `#B027DE` | matteblack.nvim `magenta` |
| storage | `#EFBF04` | matteblack.nvim `gold` |
| network | `#BF9903` | matteblack.nvim `ochre` |
| ok | `#10B981` | matteblack.nvim `teal` (its `DiagnosticOk`) |
| warn | `#D97706` | matteblack.nvim `amber` (its `DiagnosticWarn`) |
| crit | `#F87171` | substitution, below |
| unavailable | `#8A8A8D` | Omarchy `light_foreground`; matteblack.nvim `comment` |

Substitutions:

- accent: Omarchy's `accent` `#E68E0D` is the same amber as warn (ΔE00 7.4, hue 6° apart), and every other amber, orange or yellow in either file is within 26° of warn's hue, short of the 45° the check needs. The only color in either file that passes the accent checks is matteblack.nvim `blue` `#3B82F6` (ΔE00 56.1 from the upstream accent; 4.53:1 on surface). Matte Black's amber appears only as warn.
- crit: matteblack.nvim `crimson` `#DC2626` (its `DiagnosticError`) reaches only 3.45:1 on surface, and Omarchy `red` `#D35F5F` 4.42:1. The nearest passing color is matteblack.nvim `pink` `#F87171` (ΔE00 17.2), 6.03:1.

## Lupine

- Source: Omarchy's Lupine theme, [`themes/lupine/colors.toml`](https://github.com/omacom/omarchy/blob/v4.5.0/themes/lupine/colors.toml) in [omacom/omarchy](https://github.com/omacom/omarchy) at tag v4.5.0 (commit `ac2690c`).
- License: MIT, Copyright (c) David Heinemeier Hansson.

| Role | Hex | Upstream |
| --- | --- | --- |
| background | `#FAFAFA` | `background` |
| surface | `#F5F5F5` | `lighter_background`, which despite its name is a step darker than `background` (1.05:1) |
| border | `#DEDEDE` | `darker_background` |
| text | `#212121` | `foreground` |
| muted | `#424242` | `light_foreground` |
| accent | `#3264EB` | `accent`, `blue` |
| cpu | `#0C67DE` | `cyan` |
| memory | `#8A4AD7` | `magenta` |
| storage | `#013A6F` | `brown` |
| network | `#C900C4` | `red` |
| ok | `#007E3F` | substitution, below |
| warn | `#A45A00` | substitution, below |
| crit | `#DC001C` | substitution, below |
| unavailable | `#707070` | substitution, below |

Lupine is a grey-and-blue palette: its `green` `#4A2FD0` is indigo, its `yellow` and `orange` `#026FDE` are blue, and its `red` `#C900C4` is magenta. No color in the file is a green, an amber or a red, so ok, warn and crit cannot come from it, and those three substitutions are the only colors in this file that are not Lupine's. Each is new, drawn at Lupine's own lightness (its accent, `cyan`, `magenta` and `red` sit at L\* 46 to 48) with the full saturation sRGB allows at its hue, like Lupine's pure, one-channel-off colors, and passes every check on surface with a little room. Surface is `lighter_background` rather than `dark_background` `#ECECEC` because the upstream accent reaches only 4.28:1 on `#ECECEC`.

Substitutions:

- ok: Lupine's `green` `#4A2FD0` is indigo, 12° of hue from the accent. It is replaced by a green, `#007E3F` (L\* 46, hue 150°; ΔE00 52.8 from `green`), 4.75:1 on surface; against the accent ΔE00 50.6 and 143°.
- warn: Lupine's `yellow` and `orange` `#026FDE` are blue, 9° of hue from the accent. They are replaced by an amber, `#A45A00` (L\* 46, hue 65°; ΔE00 51.2), 4.77:1 on surface; against the accent ΔE00 52.9 and 132°.
- crit: Lupine's `red` `#C900C4` reaches 4.50:1 on surface but is magenta, 36° of hue from the accent, short of the 45° the check needs; every other saturated color in the file is nearer still. It is replaced by a red, `#DC001C` (L\* 46, hue 35°; ΔE00 35.9), 4.74:1 on surface; against the accent ΔE00 46.1 and 102°, the accent's closest Verdict pair. `#C900C4` stays as the network series.
- unavailable: Lupine's `muted` `#9E9E9E` reaches only 2.46:1 on surface and `dark_foreground` `#757575` 4.23:1, and the only passing greys in the file are text and muted. It is `dark_foreground` darkened to the lightest neutral grey that passes, `#707070` (ΔE00 2.0), 4.54:1.

## Flexoki Light

- Source: Omarchy's Flexoki Light theme, [`themes/flexoki-light/colors.toml`](https://github.com/omacom/omarchy/blob/v4.5.0/themes/flexoki-light/colors.toml) in [omacom/omarchy](https://github.com/omacom/omarchy) at tag v4.5.0 (commit `ac2690c`), and its upstream, [kepano/flexoki](https://github.com/kepano/flexoki): the full palette in [`css/flexoki.css`](https://github.com/kepano/flexoki/blob/2.0.0/css/flexoki.css) and the light-theme roles in its README, at tag 2.0.0 (commit `2746fe8`). Shades are named as in Flexoki, `base-50` to `base-950` for the greys and `red-400` and so on for the colors.
- Licenses: Omarchy is MIT, Copyright (c) David Heinemeier Hansson; Flexoki is MIT, Copyright (c) 2023 Steph Ango.

| Role | Hex | Upstream |
| --- | --- | --- |
| background | `#FFFCF0` | Omarchy `background`; Flexoki `paper` (`bg`) |
| surface | `#F2F0E5` | Flexoki `base-50` (`bg-2`); Omarchy `dark_background` `#F2EFE4` is the same step (ΔE00 0.5) |
| border | `#DAD8CE` | Flexoki `base-150` (`ui-2`) |
| text | `#100F0F` | Omarchy `foreground`; Flexoki `black` (`tx`) |
| muted | `#403E3C` | Omarchy `light_foreground`; Flexoki `base-800` |
| accent | `#205EA6` | Omarchy `accent`, `blue`; Flexoki `blue-600` |
| cpu | `#2F968D` | substitution, below |
| memory | `#CE5D97` | Omarchy `magenta`; Flexoki `magenta-400` |
| storage | `#AD8301` | substitution, below |
| network | `#CB6120` | substitution, below |
| ok | `#536907` | substitution, below |
| warn | `#664D01` | substitution, below |
| crit | `#C03E35` | substitution, below |
| unavailable | `#575653` | substitution, below |

Omarchy's file takes most colors from Flexoki's 400 shades, which Flexoki meant for dark backgrounds. Where one fails a check on surface, the role takes the nearest passing shade of the same Flexoki hue, rather than the nearest passing color of any hue, which for warn would be `green-700`, the ok color. The border is `base-150` rather than Omarchy's `lighter_background`, Flexoki `base-100` (`ui`) `#E6E4D9`, because `base-100` is only 1.12:1 against surface, quieter than Ink & bone's 1.22:1; `base-150` is 1.25:1. Muted is `light_foreground` rather than Flexoki's own muted text, `tx-2` (`base-600`) `#6F6E69`, which reaches only 4.47:1 on surface. Muted and unavailable are ΔE00 8.1 apart.

Substitutions:

- cpu: Omarchy `cyan` `#3AA99F` (`cyan-400`) reaches only 2.50:1 on surface. It is replaced by `cyan-500` `#2F968D` (ΔE00 6.0), 3.13:1.
- storage: Omarchy `yellow` `#D0A215` (`yellow-400`) reaches only 2.07:1 on surface, and `yellow-500` 2.52:1. It is replaced by `yellow-600` `#AD8301` (ΔE00 10.1), 3.05:1.
- network: Omarchy `orange` `#D0772B` reaches only 2.89:1 on surface. It is not itself a Flexoki shade; the nearest, `orange-400` `#DA702C` (ΔE00 4.3), reaches 2.90:1. It is replaced by `orange-500` `#CB6120` (ΔE00 7.3 from Omarchy's `orange`), 3.49:1.
- ok: Omarchy `green` `#879A39` (`green-400`) reaches only 2.74:1 on surface, and `green-600` 3.94:1. It is replaced by `green-700` `#536907` (ΔE00 19.2), 5.42:1.
- warn: Omarchy `yellow` `#D0A215` (`yellow-400`) reaches only 2.07:1 on surface, and `yellow-700` `#8E6B01` 4.32:1. It is replaced by `yellow-800` `#664D01` (ΔE00 35.3), 6.99:1. It sits ΔE00 17.3 from ok, the closest ok and warn of any theme (Catppuccin Mocha's are 20.9); the passing orange, `orange-700` `#9D4310`, would sit only ΔE00 12.9 from crit instead. Verdicts are always named in words.
- crit: Omarchy `red` `#D14D41` (`red-400`) reaches only 3.79:1 on surface. It is replaced by `red-500` `#C03E35` (ΔE00 5.4), 4.61:1; against the accent ΔE00 42.7 and 115°, the accent's closest Verdict pair.
- unavailable: Omarchy `dark_foreground` `#878580` (`base-500`) reaches only 3.22:1 on surface, and its `muted` `#B7B5AC` (`base-300`, Flexoki `tx-3`) 1.80:1. The nearest passing Flexoki grey is `base-700` `#575653` (ΔE00 18.3), 6.42:1.
