import json
from pathlib import Path
import re


ROOT = Path(__file__).resolve().parent
DIRECTIONS = [
    ("01-astra", "Interval", "Astra · high", "Open frames · warm ink · lilac", "A simple, compact mark and calm rows. The most restrained option.", "Abstract framing needs the wordmark at introduction.", "phone"),
    ("02-astra", "Register", "Astra · high", "Measurement brackets · charcoal · pale blue", "The clearest instrument-like identity, with plain measurement rows.", "Bracket geometry feels familiar and less distinctive.", "screen"),
    ("03-fable", "Level / periwinkle", "Fable 5.2 · high", "Spirit level · ink & bone · periwinkle", "Dense resource views, useful sparklines, and a complete visual state system.", "The capsule may read as a toggle; subtle state shapes need labels.", "phone"),
    ("04-fable", "Level / ivory", "Fable 5.2 · high", "Spirit level · graphite · ivory", "An engraved-instrument look with threshold ticks and a quiet warm accent.", "The wide mark reads small in a launcher and can resemble a pill.", "screen"),
]


def first_element(source, class_name):
    match = re.search(r'<(div|section)\b[^>]*class="[^"]*\b' + class_name + r'\b[^"]*"[^>]*>', source)
    if match is None:
        raise ValueError(f"No {class_name} in board")
    tag = match.group(1)
    depth = 0
    for token in re.finditer(r'</?' + tag + r'\b[^>]*>', source[match.start():]):
        depth += -1 if token.group().startswith("</") else 1
        if depth == 0:
            return source[match.start():match.start() + token.end()]
    raise ValueError("Unclosed screen element")


data = []
for folder, name, model, colors, description, caveat, screen_class in DIRECTIONS:
    path = ROOT / folder
    concept = json.loads((path / "concept.json").read_text())
    palette = {token["role"]: token["hex"] for token in concept["palette"]}
    board = (path / "board.html").read_text()
    css = "\n".join(re.findall(r"<style[^>]*>(.*?)</style>", board, re.S))
    defs = "\n".join(re.findall(r"<svg\b[^>]*>\s*<defs>.*?</defs>\s*</svg>", board, re.S))
    screen = first_element(board, screen_class)
    thumb = (
        '<!doctype html><html><head><meta charset="utf-8"><style>'
        + css
        + '\nhtml,body{margin:0!important;padding:0!important;background:' + palette["background"] + '!important}'
        + '.board,.fb3{width:350px!important;max-width:none!important;margin:0!important;padding:0!important;display:block!important}'
        + '</style></head><body><div class="board fb3">'
        + defs + screen + '</div></body></html>'
    )
    data.append(dict(
        id=folder, name=name, model=model, colors=colors, description=description,
        caveat=caveat, palette=concept["palette"], board=board, thumb=thumb,
        wordmark=(path / "wordmark.svg").read_text(),
        symbol=(path / "symbol.svg").read_text(),
    ))

template = (ROOT / "comparison-template.html").read_text()
serialized = json.dumps(data, ensure_ascii=False).replace("<", "\\u003c")
output = '<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Anshin brand directions</title></head><body>' + template.replace("__CONCEPT_DATA__", serialized) + '</body></html>'
(ROOT / "comparison.html").write_text(output)
print(f"Built {ROOT / 'comparison.html'} with {len(data)} complete concepts.")
