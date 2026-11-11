import json
from pathlib import Path
import re
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parent
COLORS = json.loads((ROOT / "palette.json").read_text())["colors"]
SVG = "http://www.w3.org/2000/svg"
ANDROID = "http://schemas.android.com/apk/res/android"
SYMBOL = (ROOT / "symbol.svg").read_text()
WORDMARK = (ROOT / "wordmark.svg").read_text()


def inner(svg):
    return svg[svg.index(">") + 1:svg.rindex("</svg>")].strip()


def write(name, content):
    path = ROOT / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content.strip() + "\n")


def symbol(color, size=24):
    return f'<svg viewBox="0 0 24 24" width="{size}" height="{size}" color="{color}" aria-hidden="true">{inner(SYMBOL)}</svg>'


mark = inner(SYMBOL)
letters = inner(WORDMARK)
lockup = f'''<svg xmlns="{SVG}" viewBox="0 0 134 26" role="img" aria-label="anshin">
<g transform="translate(1 0)" color="{COLORS['accent']}">{mark}</g>
<g color="{COLORS['text']}">{letters}</g>
</svg>'''
write("lockup.svg", lockup)
write("monochrome.svg", SYMBOL.replace(COLORS["accent"], "#000000"))
write("icon-foreground.svg", f'<svg xmlns="{SVG}" viewBox="0 0 108 108" color="{COLORS["accent"]}" role="img" aria-label="Anshin launcher foreground"><g transform="translate(30 30) scale(2)">{mark}</g></svg>')

adaptive = f'''<svg xmlns="{SVG}" viewBox="0 0 344 132" role="img" aria-label="Anshin adaptive icon construction and mask previews">
<defs><clipPath id="round"><circle cx="54" cy="54" r="36"/></clipPath><clipPath id="square"><rect x="18" y="18" width="72" height="72" rx="20"/></clipPath></defs>'''
for x, clip in [(0, ""), (118, ' clip-path="url(#round)"'), (236, ' clip-path="url(#square)"')]:
    adaptive += f'<g transform="translate({x} 0)"><g{clip}><rect width="108" height="108" fill="{COLORS["background"]}"/><g transform="translate(30 30) scale(2)" color="{COLORS["accent"]}">{mark}</g></g>'
    if x == 0:
        adaptive += f'<circle cx="54" cy="54" r="33" fill="none" stroke="{COLORS["muted"]}" stroke-width=".6" stroke-dasharray="2 2"/>'
    adaptive += '</g>'
adaptive += f'<g fill="{COLORS["muted"]}" font-family="sans-serif" font-size="9"><text x="5" y="124">108 grid / 66 safe circle</text><text x="145" y="124">Circle mask</text><text x="259" y="124">Rounded mask</text></g></svg>'
write("adaptive-icon.svg", adaptive)

source_path = ET.fromstring(SYMBOL).find(f"{{{SVG}}}path")
path_data = source_path.attrib["d"]
stroke_width = source_path.attrib["stroke-width"]
for name, viewport, group, color in [
    ("drawable/ic_anshin.xml", 24, "", "@color/anshin_accent"),
    ("drawable/ic_launcher_foreground.xml", 108, 'android:translateX="30" android:translateY="30" android:scaleX="2" android:scaleY="2"', "@color/anshin_accent"),
    ("drawable/ic_launcher_monochrome.xml", 108, 'android:translateX="30" android:translateY="30" android:scaleX="2" android:scaleY="2"', "#FF000000"),
]:
    write("android/res/" + name, f'''<vector xmlns:android="{ANDROID}" android:width="{viewport}dp" android:height="{viewport}dp" android:viewportWidth="{viewport}" android:viewportHeight="{viewport}">
  <group {group}>
    <path android:pathData="{path_data}" android:fillColor="#00000000" android:strokeColor="{color}" android:strokeWidth="{stroke_width}" android:strokeLineCap="square" android:strokeLineJoin="round"/>
  </group>
</vector>''')
write("android/res/values/anshin_colors.xml", f'<resources><color name="anshin_accent">{COLORS["accent"]}</color><color name="anshin_icon_background">{COLORS["background"]}</color></resources>')
for suffix, monochrome in [("v26", ""), ("v33", '\n  <monochrome android:drawable="@drawable/ic_launcher_monochrome"/>')]:
    write(f"android/res/mipmap-anydpi-{suffix}/ic_launcher.xml", f'''<adaptive-icon xmlns:android="{ANDROID}">
  <background android:drawable="@color/anshin_icon_background"/>
  <foreground android:drawable="@drawable/ic_launcher_foreground"/>{monochrome}
</adaptive-icon>''')

board = (ROOT / "explorations/03-fable/board.html").read_text()
board = board.replace("<title>anshin · Level</title>", "<title>Anshin · Selected design</title>")
board = board.replace("height:744px;overflow:hidden", "min-height:744px;overflow:hidden")
brand_section = re.search(r'<section class="panel brand">.*?</section>', board, re.S).group()
legend = brand_section[brand_section.index('<div class="legend">'):brand_section.rindex("</section>")]
header = f'''<section class="panel brand">
<div>{lockup.replace('viewBox=', 'width="268" height="52" viewBox=', 1)}
<h1>Your systems. At peace.</h1>
<p><b>Selected direction.</b> Interval's paired open frames with Level's periwinkle palette and resource views.</p></div>
<div class="build">{symbol(COLORS['accent'], 64)}<ul><li><b>Identity</b> · paired open frames</li><li><b>Accent</b> · periwinkle {COLORS['accent']}</li><li><b>UI</b> · system sans, tabular numbers</li><li><b>Verdict glyphs below</b> · secondary information symbols</li></ul></div>
{legend}</section>'''
board = board.replace(brand_section, header)
dashboard_lockup = lockup.replace('viewBox=', 'width="120" height="26" viewBox=', 1)
board, count = re.subn(r'(<div class="appbar">)\s*<svg.*?<span class="grow"></span>', lambda m: m.group(1) + dashboard_lockup + '<span class="grow"></span>', board, count=1, flags=re.S)
assert count == 1
system = f'''<section class="panel sys"><div>
<p class="eyebrow">Interval mark · launcher and monochrome</p>
<div class="sizes"><div><span class="tile" style="display:grid">{symbol(COLORS['accent'])}</span>24 px</div><div><span class="tile" style="display:grid">{symbol(COLORS['text'])}</span>one colour</div><div><span class="tile" style="display:grid;background:var(--text)">{symbol(COLORS['background'])}</span>reversed</div></div>
<div class="adapt">{adaptive}</div>
<p class="note">The launcher uses periwinkle on ink. Android themed icons use the monochrome layer. The frame mark always identifies Anshin; it never changes with the Verdict.</p></div>
<div><p class="eyebrow">Typography and themes</p><div class="type"><div><span>UI</span><b class="u">Machine B is Unreachable</b></div><div><span>numbers</span><b class="n">91 % · 1.2 MB/s · 48 °C</b></div><div><span>Verdict</span><b class="v">ok · warn · crit</b></div></div>
<p class="note"><b>Ink &amp; bone is the default.</b> Periwinkle identifies navigation and Live Pulses. Green, amber and red identify a current Verdict.</p>
<p class="note">Tokyo Night, Catppuccin Mocha and Matte Black are alternate themes. Their mappings need upstream palette and license verification during implementation.</p>
<p class="note">All values are illustrative. These screens are a design reference for the native Android app. Font scaling, TalkBack and touch targets still need native validation.</p></div></section>'''
board, count = re.subn(r'<section class="panel sys">.*?</section>', lambda _: system, board, count=1, flags=re.S)
assert count == 1
board = board.replace('It marks Live, selection and the bubble in the launcher icon. It never means ok.', 'It marks Live, selection and the paired-frame identity. It never means ok.')
aliases = {"background": "bg", "memory": "mem", "storage": "sto", "network": "net", "unavailable": "na"}
for role, color in COLORS.items():
    token = aliases.get(role, role)
    board = re.sub(r'(--' + token + r':)#[0-9A-Fa-f]{6}', lambda m: m.group(1) + color, board)
write("board.html", board)
print("Built selected lockup, icon assets, Android resources and design board.")
