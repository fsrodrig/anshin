# Alternate themes

Tokyo Night, Catppuccin Mocha and Matte Black, mapped onto the token roles of [Ink & bone](../palette.json). Each JSON file has the same shape as `palette.json`. They are hand-written sources: the app never loads them, and `ThemeTokensTest` fails if the Kotlin tokens drift from them. The roles and the rules they follow are in [the branding guide](../README.md#palette-and-themes).

Every value comes from the upstream file named below, at the pinned commit. Where the upstream color for a role fails a check, the role takes the nearest color (by CIEDE2000) from the same upstream palette that passes, and the substitution is listed.

## The checks

`ThemeContrastTest` computes these for every theme, Ink & bone included:

- text, muted, accent, ok, warn, crit and unavailable reach 4.5:1 (WCAG AA) against both background and surface;
- cpu, memory, storage and network reach 3:1 against both;
- the accent stays apart from ok, warn and crit: CIEDE2000 ΔE00 ≥ 20 and a CIELCh hue difference ≥ 45° against each, and chroma C\*ab ≥ 30 so it reads as a color, not as text. Ink & bone's closest pair is accent and crit at ΔE00 37.6 and 100°.

Two choices are not checked by a test but follow Ink & bone's hierarchy: surface sits one quiet step above background (Ink & bone is 1.07:1), and border is a quiet rule against surface (Ink & bone is 1.22:1). Unavailable is a grey (C\*ab ≤ 15), so an absent value never looks like a series or a Verdict.

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
