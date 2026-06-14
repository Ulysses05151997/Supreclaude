#!/usr/bin/env bash
# One-shot environment setup for the Divine Interface clip pipeline.
set -e

echo ">> Installing Python dependencies..."
pip install -r requirements.txt

echo ">> Installing espeak-ng (offline TTS fallback)..."
if command -v apt-get >/dev/null 2>&1; then
  sudo apt-get install -y espeak-ng || apt-get install -y espeak-ng || true
fi

VOICE_DIR="assets/voices"
VOICE="$VOICE_DIR/en_US-lessac-medium.onnx"
mkdir -p "$VOICE_DIR"
if [ ! -f "$VOICE" ]; then
  echo ">> Downloading Piper neural voice (lessac medium)..."
  BASE="https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_US/lessac/medium"
  curl -fsSL "$BASE/en_US-lessac-medium.onnx"      -o "$VOICE"
  curl -fsSL "$BASE/en_US-lessac-medium.onnx.json" -o "$VOICE.json"
fi

echo ">> Done. Try:"
echo "   python gen_placeholders.py"
echo "   python make_clips.py content/divine_interface.json"
