"""
Divine Interface — book-excerpt-to-video clip builder.

Reads a JSON "script" describing scenes (image + narration + caption) and
renders one MP4 per scene plus an optional stitched reel.

  python make_clips.py content/script.json
  python make_clips.py content/script.json --no-stitch
  python make_clips.py content/script.json --scene 3      # render one scene

Each scene:
  - portrait image framed into 16:9 over a blurred, darkened fill of itself
  - slow Ken Burns zoom
  - the book excerpt as a wrapped caption in a bottom gradient band
  - optional character name/title top-left
  - TTS voiceover of the narration, with the scene timed to the voice
  - crossfades between scenes, optional background music bed

Schema: see content/script.example.json
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile

import numpy as np
from PIL import Image, ImageDraw, ImageFilter, ImageFont

from moviepy import (
    AudioFileClip,
    CompositeAudioClip,
    CompositeVideoClip,
    ImageClip,
    concatenate_videoclips,
    afx,
    vfx,
)

import tts

HERE = os.path.dirname(os.path.abspath(__file__))
FONTS = os.path.join(HERE, "assets", "fonts")


def _font(*candidates):
    for c in candidates:
        if os.path.exists(c):
            return c
    return candidates[-1]


# Elegant book serif for captions; Cinzel (Trajan-like) for title cards.
CAPTION_FONT = _font(os.path.join(FONTS, "EBGaramond-Italic.ttf"),
                     "/usr/share/fonts/truetype/liberation/LiberationSerif-Italic.ttf")
CAPTION_FONT_R = _font(os.path.join(FONTS, "EBGaramond.ttf"),
                       "/usr/share/fonts/truetype/dejavu/DejaVuSerif.ttf")
TITLE_FONT = _font(os.path.join(FONTS, "Cinzel.ttf"),
                   "/usr/share/fonts/truetype/dejavu/DejaVuSerif-Bold.ttf")
GOLD = (216, 188, 130, 255)


# --------------------------------------------------------------------------- #
# image / text composition (Pillow)
# --------------------------------------------------------------------------- #
def _cover(img: Image.Image, w: int, h: int) -> Image.Image:
    """Scale+crop img to exactly w x h (cover)."""
    scale = max(w / img.width, h / img.height)
    nw, nh = int(img.width * scale + 1), int(img.height * scale + 1)
    img = img.resize((nw, nh), Image.LANCZOS)
    left, top = (nw - w) // 2, (nh - h) // 2
    return img.crop((left, top, left + w, top + h))


def _contain(img: Image.Image, w: int, h: int) -> Image.Image:
    """Scale img to fit inside w x h (contain), keep aspect."""
    scale = min(w / img.width, h / img.height)
    return img.resize((max(1, int(img.width * scale)),
                       max(1, int(img.height * scale))), Image.LANCZOS)


def compose_frame(image_path: str, w: int, h: int) -> Image.Image:
    """Portrait -> 16:9 frame: blurred darkened fill + centered sharp portrait."""
    src = Image.open(image_path).convert("RGB")

    bg = _cover(src, w, h).filter(ImageFilter.GaussianBlur(40))
    bg = Image.blend(bg, Image.new("RGB", (w, h), (10, 8, 6)), 0.45)

    fg = _contain(src, int(w * 0.62), int(h * 0.96))
    canvas = bg.copy()
    canvas.paste(fg, ((w - fg.width) // 2, (h - fg.height) // 2))
    return canvas


def _wrap(draw, text, font, max_w):
    words, lines, cur = text.split(), [], ""
    for word in words:
        trial = f"{cur} {word}".strip()
        if draw.textlength(trial, font=font) <= max_w:
            cur = trial
        else:
            if cur:
                lines.append(cur)
            cur = word
    if cur:
        lines.append(cur)
    return lines


def _tracked_width(draw, text, font, tracking):
    return sum(draw.textlength(ch, font=font) + tracking for ch in text) - tracking


def _draw_tracked(draw, xy, text, font, fill, tracking, shadow=None):
    """Draw letter-spaced text (Pillow has no native tracking)."""
    x, y = xy
    for ch in text:
        if shadow:
            draw.text((x + shadow[1], y + shadow[1]), ch, font=font, fill=shadow[0])
        draw.text((x, y), ch, font=font, fill=fill)
        x += draw.textlength(ch, font=font) + tracking


def overlay_text(w, h, caption=None, title=None):
    """Transparent RGBA overlay: bottom caption band + optional centered title."""
    ov = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    draw = ImageDraw.Draw(ov)

    if caption:
        cfont = ImageFont.truetype(CAPTION_FONT, int(h * 0.046))
        max_w = int(w * 0.78)
        text = f"“{caption}”"  # curly quotes for a literary feel
        lines = _wrap(draw, text, cfont, max_w)
        line_h = int(cfont.size * 1.34)
        block_h = line_h * len(lines)
        band_top = h - block_h - int(h * 0.13)

        # gradient band so text is readable over any image
        band = Image.new("RGBA", (w, h - band_top), (0, 0, 0, 0))
        bdraw = ImageDraw.Draw(band)
        bh = band.height
        for y in range(bh):
            a = int(215 * (y / bh) ** 0.55)
            bdraw.line([(0, y), (w, y)], fill=(0, 0, 0, a))
        ov.alpha_composite(band, (0, band_top))

        y = h - block_h - int(h * 0.075)
        for line in lines:
            lw = draw.textlength(line, font=cfont)
            x = (w - lw) / 2
            draw.text((x + 2, y + 3), line, font=cfont, fill=(0, 0, 0, 210))
            draw.text((x, y), line, font=cfont, fill=(238, 233, 224, 255))
            y += line_h

    if title:
        up = title.upper()
        tfont = ImageFont.truetype(TITLE_FONT, int(h * 0.044))
        tracking = int(h * 0.010)
        tw = _tracked_width(draw, up, tfont, tracking)
        x = (w - tw) / 2
        y = int(h * 0.085)
        _draw_tracked(draw, (x, y), up, tfont, GOLD, tracking,
                      shadow=((0, 0, 0, 190), 2))
        # thin gold rule under the title
        ry = y + int(tfont.size * 1.25)
        rule_w = tw * 0.62
        draw.line([((w - rule_w) / 2, ry), ((w + rule_w) / 2, ry)],
                  fill=(GOLD[0], GOLD[1], GOLD[2], 150), width=2)

    return ov


def _img_clip(pil_img: Image.Image, duration: float) -> ImageClip:
    arr = np.array(pil_img.convert("RGB"))
    return ImageClip(arr).with_duration(duration)


def _overlay_clip(rgba: Image.Image, duration: float) -> ImageClip:
    arr = np.array(rgba)
    rgb = ImageClip(arr[:, :, :3]).with_duration(duration)
    mask = ImageClip(arr[:, :, 3] / 255.0, is_mask=True).with_duration(duration)
    return rgb.with_mask(mask)


# --------------------------------------------------------------------------- #
# scene builder
# --------------------------------------------------------------------------- #
def resolve_voice(name, cfg):
    """A scene's `voice` may be a named entry in cfg["voices"] or a path."""
    voices = cfg.get("voices", {})
    name = name or cfg.get("voice")
    if not name:
        return tts.DEFAULT_VOICE
    path = voices.get(name, name)
    if not os.path.isabs(path):
        path = os.path.join(HERE, path)
    return path if os.path.exists(path) else tts.DEFAULT_VOICE


def make_title_card(w, h, title, subtitle=None):
    """Black act-title card: centered Cinzel title, gold rule, optional subtitle."""
    img = Image.new("RGB", (w, h), (6, 5, 4))
    # faint warm radial glow center
    glow = Image.new("L", (w, h), 0)
    gd = ImageDraw.Draw(glow)
    gd.ellipse([w * 0.2, h * 0.1, w * 0.8, h * 0.9], fill=60)
    glow = glow.filter(ImageFilter.GaussianBlur(180))
    img = Image.composite(Image.new("RGB", (w, h), (28, 22, 14)), img, glow)

    draw = ImageDraw.Draw(img)
    up = (title or "").upper()
    tfont = ImageFont.truetype(TITLE_FONT, int(h * 0.072))
    tracking = int(h * 0.014)
    tw = _tracked_width(draw, up, tfont, tracking)
    x = (w - tw) / 2
    y = h * 0.40
    _draw_tracked(draw, (x, y), up, tfont, GOLD, tracking, shadow=((0, 0, 0, 220), 3))

    ry = y + tfont.size * 1.35
    rule_w = max(tw * 0.7, w * 0.18)
    draw.line([((w - rule_w) / 2, ry), ((w + rule_w) / 2, ry)],
              fill=(GOLD[0], GOLD[1], GOLD[2]), width=2)

    if subtitle:
        sfont = ImageFont.truetype(CAPTION_FONT, int(h * 0.038))
        sw = draw.textlength(subtitle, font=sfont)
        draw.text(((w - sw) / 2, ry + h * 0.04), subtitle, font=sfont,
                  fill=(214, 206, 196))
    return img


def build_scene(scene, cfg, workdir, idx):
    w, h = cfg["resolution"]
    fps = cfg["fps"]
    pad = cfg.get("tail_pad", 0.6)
    zoom = cfg.get("ken_burns", 0.06)

    # narration -> audio + duration (shared by cards and image scenes)
    narration = scene.get("narration", "").strip()
    audio = None
    if narration:
        wav = os.path.join(workdir, f"scene_{idx:02d}.wav")
        tts.synth(
            narration, wav,
            engine=cfg.get("tts", "auto"),
            voice=resolve_voice(scene.get("voice"), cfg),
            length_scale=scene.get("length_scale", cfg.get("length_scale", 1.0)),
            fx=scene.get("fx"),
        )
        audio = AudioFileClip(wav)
        duration = audio.duration + pad
    else:
        duration = float(scene.get("duration", 4.0))
    duration = max(duration, float(scene.get("min_duration", 3.0)))

    # --- act/title card scene (no portrait) ---
    if scene.get("card") or not scene.get("image"):
        card = _img_clip(make_title_card(w, h, scene.get("title"),
                                         scene.get("subtitle")), duration)
        card = card.resized(lambda t: 1 + 0.02 * (t / duration))
        scene_clip = CompositeVideoClip([card], size=(w, h)).with_duration(duration)
        scene_clip = scene_clip.with_effects(
            [vfx.CrossFadeIn(0.6), vfx.CrossFadeOut(0.6)])
        if audio is not None:
            scene_clip = scene_clip.with_audio(audio)
        return scene_clip.with_fps(fps)

    # --- portrait scene ---
    img_path = scene["image"]
    if not os.path.isabs(img_path):
        img_path = os.path.join(HERE, img_path)
    if not os.path.exists(img_path):
        raise FileNotFoundError(f"scene {idx}: image not found: {img_path}")

    # base frame + ken burns zoom-in (cropped to frame by the composite)
    base = _img_clip(compose_frame(img_path, w, h), duration)
    base = base.resized(lambda t: 1 + zoom * (t / duration))

    # caption defaults to the narration text
    caption = scene.get("caption", narration if scene.get("show_text", True) else "")
    title = scene.get("title")
    ov = _overlay_clip(overlay_text(w, h, caption=caption, title=title), duration)
    ov = ov.with_effects([vfx.CrossFadeIn(0.4), vfx.CrossFadeOut(0.4)])

    scene_clip = CompositeVideoClip([base, ov], size=(w, h)).with_duration(duration)
    if audio is not None:
        scene_clip = scene_clip.with_audio(audio)
    return scene_clip.with_fps(fps)


def add_music(video, cfg):
    music = cfg.get("music")
    if not music:
        return video
    if not os.path.isabs(music):
        music = os.path.join(HERE, music)
    if not os.path.exists(music):
        print(f"  (music not found, skipping: {music})")
        return video

    vol = cfg.get("music_volume", 0.12)
    bed = AudioFileClip(music).with_effects(
        [afx.AudioLoop(duration=video.duration), afx.MultiplyVolume(vol)]
    )
    tracks = [bed] if video.audio is None else [video.audio, bed]
    return video.with_audio(CompositeAudioClip(tracks))


# --------------------------------------------------------------------------- #
# main
# --------------------------------------------------------------------------- #
def main():
    ap = argparse.ArgumentParser(description="Build video clips from a book script.")
    ap.add_argument("script", help="path to script JSON")
    ap.add_argument("--scene", type=int, help="render only this scene index (1-based)")
    ap.add_argument("--no-stitch", action="store_true", help="skip the combined reel")
    ap.add_argument("--outdir", default="output")
    args = ap.parse_args()

    with open(args.script, encoding="utf-8") as fh:
        cfg = json.load(fh)

    cfg.setdefault("resolution", [1920, 1080])
    cfg.setdefault("fps", 30)
    cfg["resolution"] = list(cfg["resolution"])

    outdir = os.path.join(HERE, args.outdir)
    clips_dir = os.path.join(outdir, "clips")
    os.makedirs(clips_dir, exist_ok=True)

    scenes = cfg["scenes"]
    cross = cfg.get("transition", 0.7)

    with tempfile.TemporaryDirectory() as workdir:
        rendered, built = [], []
        for i, scene in enumerate(scenes, 1):
            if args.scene and i != args.scene:
                continue
            print(f"[scene {i}/{len(scenes)}] {scene.get('title') or scene['image']}")
            clip = build_scene(scene, cfg, workdir, i)
            built.append(clip)

            slug = scene.get("id") or f"scene_{i:02d}"
            path = os.path.join(clips_dir, f"{slug}.mp4")
            clip.write_videofile(
                path, fps=cfg["fps"], codec="libx264", audio_codec="aac",
                preset="medium", logger=None,
            )
            rendered.append(path)
            print(f"    -> {os.path.relpath(path, HERE)}  ({clip.duration:.1f}s)")

        if not args.no_stitch and not args.scene and len(built) > 1:
            print("[stitch] building combined reel with crossfades...")
            faded = [built[0]] + [
                c.with_effects([vfx.CrossFadeIn(cross)]) for c in built[1:]
            ]
            reel = concatenate_videoclips(faded, method="compose", padding=-cross)
            reel = add_music(reel, cfg)
            title = cfg.get("title", "reel").lower().replace(" ", "_")
            reel_path = os.path.join(outdir, f"{title}.mp4")
            reel.write_videofile(
                reel_path, fps=cfg["fps"], codec="libx264", audio_codec="aac",
                preset="medium", logger=None,
            )
            print(f"    -> {os.path.relpath(reel_path, HERE)}  ({reel.duration:.1f}s)")

    print("\nDone. Rendered:")
    for p in rendered:
        print("  ", os.path.relpath(p, HERE))


if __name__ == "__main__":
    sys.exit(main())
