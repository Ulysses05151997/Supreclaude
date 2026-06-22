"""
Generate stand-in 'Old Master portrait' placeholder images so the pipeline can
be demoed before the real REF_*.png character art is dropped into assets/images/.

These are NOT the real art -- just umber-toned cards with the character name, so
make_clips.py has files to frame. Replace them with the real reference images
(same filenames) when ready.
"""
import os
import math

from PIL import Image, ImageDraw, ImageFilter, ImageFont

OUT = os.path.join(os.path.dirname(__file__), "assets", "images")
os.makedirs(OUT, exist_ok=True)

SERIF_BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSerif-Bold.ttf"
if not os.path.exists(SERIF_BOLD):
    SERIF_BOLD = "/usr/share/fonts/truetype/liberation/LiberationSerif-Bold.ttf"

W, H = 832, 1216

# slug -> (display name, base color, accent for "digital" look)
CHARACTERS = {
    "amaris":            ("Sister Amaris", (74, 58, 38), None),
    "esmeralda":         ("Sister Esmeralda Doreste", (70, 56, 40), None),
    "adunni":            ("Sister Adunni Folashade", (78, 60, 40), None),
    "rebecca_santos":    ("Rebecca Santos", (64, 52, 44), None),
    "sarah_chen":        ("Dr. Sarah Chen", (60, 52, 46), None),
    "marcus":            ("Marcus Rodriguez", (66, 54, 40), None),
    "david_1":           ("David Michael Harris", (62, 52, 46), None),
    "david_2":           ("David Michael Harris", (52, 44, 40), None),
    "emily_normal":      ("Emily Catherine Parker", (70, 64, 56), None),
    "emily_crying":      ("Emily Catherine Parker", (58, 54, 50), None),
    "emily_possessed":   ("Emily Catherine Parker", (40, 38, 36), None),
    "entity_malphas":    ("Malphas", (4, 10, 4), (40, 220, 80)),
    "unknown_man":       ("The Unknown Man", (44, 40, 36), None),
    "osullivan":         ("Father O'Sullivan", (58, 52, 46), None),
    "torretti":          ("Cardinal Torretti", (70, 40, 40), None),
    "linda":             ("Linda Morrison", (78, 70, 60), None),
}


def vignette(img, strength=0.85):
    w, h = img.size
    mask = Image.new("L", (w, h), 0)
    d = ImageDraw.Draw(mask)
    d.ellipse([-w * 0.2, -h * 0.2, w * 1.2, h * 1.2], fill=255)
    mask = mask.filter(ImageFilter.GaussianBlur(180))
    dark = Image.new("RGB", (w, h), (0, 0, 0))
    return Image.composite(img, dark, Image.eval(mask, lambda v: int(v * strength + 255 * (1 - strength))))


def make_card(slug, name, base, accent):
    img = Image.new("RGB", (W, H), base)
    d = ImageDraw.Draw(img)

    if accent:  # digital / Matrix look for the entity
        font = ImageFont.truetype(SERIF_BOLD, 26)
        for y in range(0, H, 34):
            for x in range(0, W, 24):
                if (x * 13 + y * 7) % 5:
                    d.text((x, y), "01"[(x + y) // 24 % 2], font=font,
                           fill=(accent[0], accent[1], accent[2]))
        img = img.filter(ImageFilter.GaussianBlur(0.6))
        d = ImageDraw.Draw(img)
    else:
        # warm radial light from upper-left, painterly gradient
        for y in range(H):
            for_step = y / H
            r = int(base[0] * (1.15 - 0.5 * for_step))
            g = int(base[1] * (1.15 - 0.5 * for_step))
            b = int(base[2] * (1.15 - 0.5 * for_step))
            d.line([(0, y), (W, y)], fill=(min(r, 255), min(g, 255), min(b, 255)))
        # simple portrait silhouette so framing reads as a figure
        cx = W // 2
        d.ellipse([cx - 150, 300, cx + 150, 600], fill=tuple(int(c * 0.7) for c in base))
        d.polygon([(cx - 230, H), (cx - 170, 640), (cx + 170, 640), (cx + 230, H)],
                  fill=tuple(int(c * 0.62) for c in base))
        img = img.filter(ImageFilter.GaussianBlur(8))
        img = vignette(img, 0.8)
        d = ImageDraw.Draw(img)

    # name plate
    nfont = ImageFont.truetype(SERIF_BOLD, 46)
    lines = name.split(" ")
    # wrap to <= 2 words per line for a poster feel
    rows, cur = [], []
    for w_ in lines:
        cur.append(w_)
        if len(cur) == 2:
            rows.append(" ".join(cur)); cur = []
    if cur:
        rows.append(" ".join(cur))
    y = H - 80 - len(rows) * 56
    for row in rows:
        tw = d.textlength(row, font=nfont)
        x = (W - tw) / 2
        d.text((x + 2, y + 2), row, font=nfont, fill=(0, 0, 0))
        d.text((x, y), row, font=nfont, fill=(224, 198, 138))
        y += 56

    path = os.path.join(OUT, f"{slug}.png")
    img.save(path)
    return path


if __name__ == "__main__":
    for slug, (name, base, accent) in CHARACTERS.items():
        p = make_card(slug, name, base, accent)
        print("wrote", os.path.relpath(p, os.path.dirname(__file__)))
