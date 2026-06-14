"""
Text-to-speech for the Divine Interface clip pipeline.

Three backends, tried in order for engine="auto":
  1. piper   - offline neural voice (best quality, needs a .onnx voice model)
  2. gtts    - Google Translate TTS (good voice, needs network, can rate-limit)
  3. espeak  - offline robotic fallback (always available)

Every backend returns a 16-bit PCM WAV path so MoviePy reads it uniformly.
"""
from __future__ import annotations

import os
import shutil
import subprocess
import tempfile

from imageio_ffmpeg import get_ffmpeg_exe

FFMPEG = get_ffmpeg_exe()
PIPER_BIN = shutil.which("piper")
ESPEAK_BIN = shutil.which("espeak-ng") or shutil.which("espeak")

DEFAULT_VOICE = os.path.join(
    os.path.dirname(__file__), "assets", "voices", "en_US-lessac-medium.onnx"
)


def _to_wav(src: str, dst: str) -> bool:
    """Transcode any audio file to mono 22.05k 16-bit WAV."""
    try:
        subprocess.run(
            [FFMPEG, "-y", "-i", src, "-ac", "1", "-ar", "22050",
             "-sample_fmt", "s16", dst],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True,
        )
        return os.path.exists(dst) and os.path.getsize(dst) > 1000
    except Exception:
        return False


def _piper(text: str, out_wav: str, voice: str, length_scale: float) -> bool:
    if not PIPER_BIN or not voice or not os.path.exists(voice):
        return False
    try:
        subprocess.run(
            [PIPER_BIN, "-m", voice, "-f", out_wav,
             "--length-scale", str(length_scale)],
            input=text.encode("utf-8"),
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True,
        )
        return os.path.exists(out_wav) and os.path.getsize(out_wav) > 1000
    except Exception:
        return False


def _gtts(text: str, out_wav: str) -> bool:
    try:
        from gtts import gTTS
    except Exception:
        return False
    try:
        tmp_mp3 = out_wav + ".mp3"
        gTTS(text).save(tmp_mp3)
        ok = _to_wav(tmp_mp3, out_wav)
        if os.path.exists(tmp_mp3):
            os.remove(tmp_mp3)
        return ok
    except Exception:
        return False


def _espeak(text: str, out_wav: str, wpm: int = 150) -> bool:
    if not ESPEAK_BIN:
        return False
    try:
        raw = out_wav + ".raw.wav"
        subprocess.run(
            [ESPEAK_BIN, "-s", str(wpm), "-w", raw, text],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True,
        )
        ok = _to_wav(raw, out_wav)
        if os.path.exists(raw):
            os.remove(raw)
        return ok
    except Exception:
        return False


def synth(
    text: str,
    out_wav: str,
    engine: str = "auto",
    voice: str = DEFAULT_VOICE,
    length_scale: float = 1.0,
) -> str:
    """
    Synthesize `text` to `out_wav`. `length_scale` > 1 slows piper down
    (good for a measured, narrated feel). Returns the wav path.
    Raises RuntimeError if every backend fails.
    """
    text = (text or "").strip()
    if not text:
        raise ValueError("synth() got empty text")

    order = {
        "auto": ["piper", "gtts", "espeak"],
        "piper": ["piper"],
        "gtts": ["gtts"],
        "espeak": ["espeak"],
    }.get(engine, ["piper", "gtts", "espeak"])

    for name in order:
        if name == "piper" and _piper(text, out_wav, voice, length_scale):
            return out_wav
        if name == "gtts" and _gtts(text, out_wav):
            return out_wav
        if name == "espeak" and _espeak(text, out_wav):
            return out_wav

    raise RuntimeError(
        f"All TTS backends failed for engine={engine!r}. "
        f"piper={bool(PIPER_BIN)} espeak={bool(ESPEAK_BIN)}"
    )


if __name__ == "__main__":
    import sys

    txt = sys.argv[1] if len(sys.argv) > 1 else "The rain had not stopped for three days."
    out = sys.argv[2] if len(sys.argv) > 2 else "/tmp/tts_test.wav"
    print("wrote", synth(txt, out))
