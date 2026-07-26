#!/usr/bin/env bash
#
# Regenerate the embedded Steam appid -> Discord application id table.
#
# Discord publishes the list of games it can detect, and each entry carries the
# third-party SKUs it ships under. Taking the Steam ones gives us a direct
# appid -> Discord app id map, which is what lets the Rich Presence headline read
# "Playing Unturned" instead of the name of our own app.
#
# The table is EMBEDDED rather than fetched at runtime on purpose: Discord is
# blocked in some of the countries glow is most used in, so an app that had to
# reach discord.com would simply fail there. It also means no startup network
# call at all, which keeps the binary's behaviour boring for antivirus.
#
# Run this now and then (a release cadence is plenty) and commit the result.
#
# Usage: bash scripts/gen-steam-map.sh
#
set -euo pipefail
cd "$(dirname "$0")/.."

SRC="https://discord.com/api/v10/applications/detectable"
OUT="internal/steam/steam-apps.bin.gz"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "▸ Fetching the detectable-games list"
curl -fsS --compressed --max-time 120 "$SRC" -o "$TMP/detectable.json"
echo "  $(wc -c < "$TMP/detectable.json" | awk '{printf "%.1f MB", $1/1048576}') downloaded"

python3 - "$TMP/detectable.json" "$OUT" <<'PY'
import gzip, json, struct, sys

src, out = sys.argv[1], sys.argv[2]
games = json.load(open(src, encoding="utf-8"))

pairs = {}
for g in games:
    try:
        discord_id = int(g["id"])
    except (KeyError, TypeError, ValueError):
        continue
    for sku in (g.get("third_party_skus") or []):
        if sku.get("distributor") != "steam":
            continue
        try:
            appid = int(sku["id"])
        except (KeyError, TypeError, ValueError):
            continue
        # An appid can appear on several entries (editions, betas). First wins,
        # which is the one Discord lists first and therefore the main app.
        pairs.setdefault(appid, discord_id)

# Sorted fixed-width records so the reader can binary-search without unpacking
# the whole table: uint32 appid + uint64 discord id, little endian.
blob = b"".join(struct.pack("<IQ", a, d) for a, d in sorted(pairs.items()))
with gzip.open(out, "wb", compresslevel=9) as f:
    f.write(blob)

print(f"  {len(pairs)} games mapped")
PY

echo "▸ Wrote $OUT ($(wc -c < "$OUT" | awk '{printf "%.0f KB", $1/1024}'))"
echo "▸ Commit it so the table ships with the build."
