#!/usr/bin/env bash
#
# Record the Sympozium demo walkthrough and convert to GIF.
#
# Prerequisites:
#   - Dev server running (npm run dev)
#   - Kubernetes cluster with Sympozium operator running
#   - ffmpeg installed (brew install ffmpeg)
#   - Optional: gifsicle for optimization (brew install gifsicle)
#
# Usage:
#   ./scripts/record-demo.sh
#   CYPRESS_BASE_URL=http://localhost:3000 ./scripts/record-demo.sh
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WEB_DIR="$(dirname "$SCRIPT_DIR")"
cd "$WEB_DIR"

VIDEO_DIR="cypress/videos"
SPEC="cypress/e2e/demo-walkthrough.cy.ts"
OUTPUT_GIF="demo.gif"
OUTPUT_MP4="demo.mp4"

# Auto-detect API token from k8s secret if not already set.
if [[ -z "${CYPRESS_API_TOKEN:-}" ]]; then
  CYPRESS_API_TOKEN=$(
    kubectl get secret -n sympozium-system sympozium-ui-token \
      -o jsonpath='{.data.token}' 2>/dev/null | base64 -d 2>/dev/null
  ) || true
  if [[ -n "$CYPRESS_API_TOKEN" ]]; then
    echo "▸ Auto-detected API token from sympozium-ui-token secret"
    export CYPRESS_API_TOKEN
  else
    echo "⚠ No CYPRESS_API_TOKEN found — login page will block the demo"
  fi
fi

# Cypress 15 ignores --config video=true from CLI, so we use a dedicated config.
DEMO_CONFIG="cypress.config.demo.ts"
cat > "$DEMO_CONFIG" <<'TSEOF'
import { defineConfig } from "cypress";
export default defineConfig({
  e2e: {
    baseUrl: process.env.CYPRESS_BASE_URL || "http://localhost:5173",
    supportFile: "cypress/support/e2e.ts",
    specPattern: "cypress/e2e/**/*.cy.ts",
    viewportWidth: 1280,
    viewportHeight: 800,
    defaultCommandTimeout: 15000,
    video: true,
    videoCompression: false,
  },
});
TSEOF

echo "▸ Running Cypress demo spec with video recording..."
npx cypress run \
  --spec "$SPEC" \
  --config-file "$DEMO_CONFIG" \
  --browser chrome \
  --no-runner-ui \
  --env "API_TOKEN=$CYPRESS_API_TOKEN" \
  || true  # Don't fail if assertions flap — we want the video regardless.

rm -f "$DEMO_CONFIG"

# Find the recorded video.
VIDEO_FILE="$VIDEO_DIR/demo-walkthrough.cy.ts.mp4"
if [[ ! -f "$VIDEO_FILE" ]]; then
  echo "✗ Video not found at $VIDEO_FILE"
  echo "  Check $VIDEO_DIR/ for available files."
  ls -la "$VIDEO_DIR/" 2>/dev/null || true
  exit 1
fi

echo "▸ Video recorded: $VIDEO_FILE"

# Trim: skip first ~0.8s (white flash before CSS loads) and last ~1.4s (extra tail).
DURATION=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$VIDEO_FILE")
END=$(echo "$DURATION - 1.4" | bc)
ffmpeg -y -ss 0.8 -to "$END" -i "$VIDEO_FILE" -c copy "$OUTPUT_MP4" 2>/dev/null
echo "▸ Saved trimmed MP4: $OUTPUT_MP4"

# Convert to GIF via ffmpeg (high quality, max ~5MB).
#   - Crop 134px gray borders from Cypress headless Chrome
#   - fps=12, 860px width, 210 colors, floyd_steinberg dithering
#   - palettegen/paletteuse for accurate color reproduction
echo "▸ Converting to GIF..."
PALETTE=$(mktemp /tmp/palette-XXXXXX.png)
CROP="crop=1013:632:134:0"
ffmpeg -y -i "$OUTPUT_MP4" \
  -vf "${CROP},fps=12,scale=860:-1:flags=lanczos,palettegen=max_colors=210:stats_mode=diff" \
  "$PALETTE" 2>/dev/null

ffmpeg -y -i "$OUTPUT_MP4" -i "$PALETTE" \
  -lavfi "${CROP},fps=12,scale=860:-1:flags=lanczos [x]; [x][1:v] paletteuse=dither=floyd_steinberg:diff_mode=rectangle" \
  "$OUTPUT_GIF" 2>/dev/null

rm -f "$PALETTE"

FILE_SIZE=$(du -h "$OUTPUT_GIF" | cut -f1)
echo ""
echo "✓ Demo GIF ready: $OUTPUT_GIF ($FILE_SIZE)"
echo "  MP4 also saved: $OUTPUT_MP4"
echo ""
echo "  To embed in README:"
echo "  ![Sympozium Demo](./web/demo.gif)"
