# Divine Interface — Book-to-Video Clip Pipeline

Turn excerpts from *Divine Interface: Faith Updated* (by Ulysses Isa) plus
character/scene art into narrated **16:9 video clips** — each with a slow Ken
Burns motion, on-screen captions, AI voiceover, crossfades, and an optional
music bed.

This does **not** generate video with an AI video model. It *assembles* real
`.mp4` files from your images + text using Pillow, MoviePy and ffmpeg, with
offline neural text-to-speech (Piper).

## What you get

- `make_clips.py` — reads a JSON "script" and renders one MP4 per scene plus a
  stitched reel.
- `tts.py` — voiceover with three backends, auto-fallback: **Piper** (offline
  neural, best) → **gTTS** (networked) → **espeak-ng** (offline robotic).
- `gen_placeholders.py` — temporary umber name-cards so you can preview the
  pipeline before the real art is in place.
- `content/divine_interface.json` — a teaser script wired to real screenplay
  lines and the cast.

## Setup

```bash
bash setup.sh          # installs deps + espeak-ng + downloads the Piper voice
```

or manually:

```bash
pip install -r requirements.txt
sudo apt-get install -y espeak-ng
# Piper voice model -> assets/voices/en_US-lessac-medium.onnx(.json)
```

## Render

```bash
python gen_placeholders.py                       # only until real art is added
python make_clips.py content/divine_interface.json
# outputs: output/clips/<id>.mp4  and  output/divine_interface_teaser.mp4
```

Useful flags: `--scene N` (render one scene), `--no-stitch` (skip the reel).

## Using your real character art

The reference images (`REF_David_Harris_Portrait_1.jpeg`, etc.) must exist as
**files** on disk — pasting them in chat is not enough for ffmpeg. Drop them in
`assets/images/` and point each scene's `"image"` at them. Portrait images are
automatically framed into 16:9 over a blurred fill of themselves, so tall
Old-Master portraits look intentional, not letterboxed.

## Script schema (`content/*.json`)

```jsonc
{
  "title": "Divine Interface Teaser",
  "resolution": [1920, 1080],
  "fps": 30,
  "tts": "auto",                 // auto | piper | gtts | espeak
  "voice": "assets/voices/en_US-lessac-medium.onnx",
  "length_scale": 1.06,          // >1 slows the narrator down
  "transition": 0.8,             // crossfade seconds between scenes
  "ken_burns": 0.06,             // zoom amount per scene
  "music": "assets/music/bed.mp3",   // optional; skipped if missing
  "music_volume": 0.10,
  "scenes": [
    {
      "id": "01_florence",
      "image": "assets/images/amaris.png",
      "title": "Florence, 1347",          // optional on-screen label
      "narration": "We write as the Great Mortality consumes our lands...",
      "caption": "optional — defaults to the narration text",
      "min_duration": 5
    }
  ]
}
```

Per-scene knobs: `narration` (spoken + timed), `caption` (overrides the
on-screen text), `title`, `min_duration`, `duration` (used when there's no
narration), `show_text: false` (image + voice only).

## Notes

- Voice: the bundled voice is `en_US-lessac-medium` (calm, narration-friendly).
  Swap in any Piper voice via the `voice` field. The entity/Malphas lines can use
  a slower `length_scale` for menace.
- Lighting/style per act (corporate fluorescent, apartment amber, server-room
  blue/green, epilogue glow-stick) lives in the source art, per the Character
  Design Reference Sheets — the pipeline preserves whatever you feed it.
