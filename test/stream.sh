#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   chmod +x stream.sh
#   ./stream.sh < file.md
#
# Streams stdin in random-sized chunks to simulate AI output.

# ---------------- config ----------------
size_min=3      # minimum chunk size, in characters
size_max=18     # maximum chunk size, in characters
delay=40        # delay between chunks, in milliseconds
# ----------------------------------------

if (( size_min < 1 || size_max < size_min )); then
  echo "Invalid config: size_min must be >= 1 and size_max >= size_min" >&2
  exit 1
fi

range=$((size_max - size_min + 1))
sleep_time="$(printf '%d.%03d' "$((delay / 1000))" "$((delay % 1000))")"

while true; do
  chunk_size=$((size_min + RANDOM % range))
  chunk=""

  # Read up to chunk_size characters from stdin.
  # At EOF, read may fail while still returning a partial final chunk.
  if IFS= read -r -N "$chunk_size" chunk; then
    printf '%s' "$chunk"
  else
    if [[ -n "$chunk" ]]; then
      printf '%s' "$chunk"
    fi
    break
  fi

  if (( delay > 0 )); then
    sleep "$sleep_time"
  fi
done