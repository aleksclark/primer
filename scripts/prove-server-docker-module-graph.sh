#!/usr/bin/env bash
# Contract: LMS / TV / ingest Dockerfiles copy every local module in the
# server graph (server/go.mod replace + workspace primer-agents runtime).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
fail=0

needles=(
	'COPY primer-agents/go.mod primer-agents/go.sum ./primer-agents/'
	'COPY primer-agents/client/go/go.mod primer-agents/client/go/go.sum ./primer-agents/client/go/'
	'COPY primer-agents/ ./primer-agents/'
	'ENV GOWORK=/src/go.work'
	'./primer-agents/client/go'
)

for f in Dockerfile Dockerfile.tv Dockerfile.ingest; do
	path="$root/$f"
	if [[ ! -f "$path" ]]; then
		echo "FAIL missing $f"
		fail=1
		continue
	fi
	for needle in "${needles[@]}"; do
		if ! grep -Fq "$needle" "$path"; then
			echo "FAIL $f missing ${needle@Q}"
			fail=1
		fi
	done
done

ingest="$root/Dockerfile.ingest"
ingest_needles=(
	'https://github.com/yt-dlp/yt-dlp/releases/download/2026.06.09/yt-dlp'
	'ENV INGEST_YTDLP_OUTPUT_DIR=/media/primer'
)
for needle in "${ingest_needles[@]}"; do
	if ! grep -Fq "$needle" "$ingest"; then
		echo "FAIL Dockerfile.ingest missing ${needle@Q}"
		fail=1
	fi
done
if grep -Fq '/releases/latest/download/yt-dlp' "$ingest" || grep -Fq 'ENV INGEST_YTDLP_OUTPUT_DIR=/media/tv/Primer' "$ingest"; then
	echo 'FAIL Dockerfile.ingest still uses /latest yt-dlp or /media/tv/Primer'
	fail=1
fi

if [[ "$fail" -ne 0 ]]; then
	exit 1
fi
echo 'OK server Dockerfiles copy local primer-agents modules and set GOWORK'
echo 'OK Dockerfile.ingest pins yt-dlp 2026.06.09 and writes /media/primer'
