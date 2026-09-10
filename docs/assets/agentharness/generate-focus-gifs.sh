#!/usr/bin/env bash
set -euo pipefail

asset_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

# Keep the SVG as the canonical source. These overlays create a short,
# deterministic walkthrough without maintaining a second set of drawings.
magick -background '#111110' "$asset_dir/execution-paths.svg" "$work_dir/base.png"

magick "$work_dir/base.png" \
  -fill 'rgba(61,214,140,0.07)' -stroke '#3dd68c' -strokewidth 5 \
  -draw 'roundrectangle 452,122 773,283 16,16' "$work_dir/chat.png"

magick "$work_dir/base.png" \
  -fill 'rgba(76,156,255,0.07)' -stroke '#4c9cff' -strokewidth 5 \
  -draw 'roundrectangle 452,297 773,458 16,16' "$work_dir/run.png"

magick "$work_dir/base.png" \
  -fill 'rgba(232,86,42,0.06)' -stroke '#e8562a' -strokewidth 5 \
  -draw 'roundrectangle 827,152 1143,378 16,16' "$work_dir/boundary.png"

magick -delay 90 "$work_dir/base.png" \
  -delay 120 "$work_dir/chat.png" \
  -delay 120 "$work_dir/run.png" \
  -delay 140 "$work_dir/boundary.png" \
  -loop 0 -layers Optimize "$asset_dir/execution-paths-focus.gif"
