#!/usr/bin/env python3
"""Read-only live acceptance: disk identity -> TV -> Jellyfin Collection -> media bytes.

Requires INGEST_JELLYFIN_BASE_URL, INGEST_JELLYFIN_API_KEY,
INGEST_JELLYFIN_USER_ID, INGEST_TV_BASE_URL, INGEST_TV_ADMIN_KEY. Credentials are sent only in headers, never printed or
passed to ffmpeg. This does not create schedules, grants, or playback sessions.
"""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def endpoint(name):
    value = os.environ.get(name, "").rstrip("/")
    require(bool(value), f"{name} is required")
    return value


class API:
    def __init__(self, base, header, key, user_id=""):
        self.base, self.headers, self.user_id = base, {header: key}, user_id

    def open(self, path, query=None, headers=None):
        url = self.base + path
        if query:
            url += "?" + urllib.parse.urlencode(query)
        return urllib.request.urlopen(urllib.request.Request(
            url, headers={**self.headers, **(headers or {})}), timeout=120)

    def get(self, path, query=None):
        with self.open(path, query) as response:
            return json.load(response)

    def items(self, query):
        # BoxSet linked children are user-scoped when Recursive=false in JF
        # 10.11; an unscoped query silently returns no direct members.
        if self.user_id:
            query = {**query, "UserId": self.user_id}
        rows = []
        for offset in range(0, 100000, 200):
            page = self.get("/Items", {**query, "StartIndex": offset, "Limit": 200})
            batch = page["Items"]
            rows.extend(batch)
            if len(batch) < 200 or len(rows) >= page["TotalRecordCount"]:
                return rows
        raise RuntimeError("Jellyfin paging guard reached")


def digest(path):
    with path.open("rb") as f:
        return hashlib.file_digest(f, "sha256").hexdigest()


def normalize_calendar_date(value):
    if not value:
        return ""
    if isinstance(value, str):
        if re.fullmatch(r"\d{8}", value):
            return f"{value[:4]}-{value[4:6]}-{value[6:8]}"
        if re.fullmatch(r"\d{4}-\d{2}-\d{2}", value):
            return value
        if value.endswith("Z"):
            value = value[:-1] + "+00:00"
        try:
            return dt.datetime.fromisoformat(value).date().isoformat()
        except ValueError:
            return ""
    if isinstance(value, (int, float)):
        return dt.datetime.fromtimestamp(value, tz=dt.timezone.utc).date().isoformat()
    return ""


def info_calendar_date(info):
    for key in ("upload_date", "release_date", "release_timestamp", "timestamp"):
        calendar = normalize_calendar_date(info.get(key))
        if calendar:
            return calendar
    raise RuntimeError("Unable to derive upload-date calendar from info.json")


def provider_id(item, key):
    ids = item.get("ProviderIds") or {}
    lowered = {str(k).lower(): v for k, v in ids.items()}
    return lowered.get(key.lower(), "")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--youtube-id", required=True, action="append", dest="ids")
    p.add_argument("--output-root", type=Path, required=True,
                   help="Host equivalent of INGEST_YTDLP_OUTPUT_DIR")
    p.add_argument("--collection", default="Primer")
    p.add_argument("--evidence", type=Path, required=True)
    args = p.parse_args()
    for video_id in args.ids:
        require(bool(re.fullmatch(r"[A-Za-z0-9_-]{11}", video_id)), "Invalid YouTube id")
    jf = API(endpoint("INGEST_JELLYFIN_BASE_URL"), "X-Emby-Token", endpoint("INGEST_JELLYFIN_API_KEY"),
             endpoint("INGEST_JELLYFIN_USER_ID"))
    tv = API(endpoint("INGEST_TV_BASE_URL"), "X-Admin-Key", endpoint("INGEST_TV_ADMIN_KEY"))
    collections = [x for x in jf.items({"IncludeItemTypes": "BoxSet", "Recursive": "true"})
                   if x["Name"] == args.collection]
    require(len(collections) == 1, "Expected exactly one named Jellyfin Collection")
    collection = collections[0]
    members = {x["Id"] for x in jf.items({"ParentId": collection["Id"], "Recursive": "false"})}
    evidence = {"collection": args.collection, "collectionId": collection["Id"],
                "collectionMembers": len(members), "videos": []}
    for video_id in args.ids:
        records = tv.get("/media-items", {"filter": "youtube_video_id:" + video_id})
        require(records["totalCount"] == 1, f"{video_id}: expected one TV media record")
        record = records["items"][0]
        slug = record["manifestSlug"]
        require(bool(re.fullmatch(r"[a-z0-9][a-z0-9-]*", slug)), "Invalid manifest slug")
        require(record["jellyfinItemId"] in members, f"{video_id}: absent from Collection")
        require(record["directPlayOk"], f"{video_id}: TV marks direct play incompatible")
        items = jf.items({"Ids": record["jellyfinItemId"], "Recursive": "true", "Fields": "Path,MediaStreams,ProviderIds,Overview,PremiereDate"})
        require(len(items) == 1, f"{video_id}: missing Jellyfin item")
        item = items[0]
        require(item["Type"] in ("Episode", "Video"), "A folder was imported instead of a video")
        require("/Shows/" + slug + "/" in item["Path"], "Jellyfin path/slug mismatch")
        season = args.output_root / "Shows" / slug / "Season 01"
        files = [f for f in season.iterdir() if f.suffix in (".mp4", ".mkv", ".webm")
                 and f.stem.endswith("[" + video_id + "]")]
        require(len(files) == 1, "Expected one finalized local media file")
        media = files[0]
        require(Path(item["Path"]).name == media.name, "Jellyfin/local filename mismatch")
        info = json.loads(media.with_suffix(".info.json").read_text())
        require(info["id"] == video_id, "Sidecar identity mismatch")
        nfo = ET.parse(media.with_suffix(".nfo"))
        require(nfo.findtext("uniqueid[@type='youtube']") == video_id, "NFO identity mismatch")
        require(nfo.findtext("uniqueid[@type='primer-slug']") == slug, "NFO slug identity mismatch")
        expected_title = (nfo.findtext("title") or "").strip()
        require(bool(expected_title), "NFO title missing")
        expected_show = (nfo.findtext("showtitle") or "").strip()
        require(bool(expected_show), "NFO showtitle missing")
        expected_plot = (nfo.findtext("plot") or "")
        expected_calendar = normalize_calendar_date(nfo.findtext("premiered")) or info_calendar_date(info)
        require(item["Name"] == expected_title, "Jellyfin title does not match authored NFO title")
        require(provider_id(item, "youtube") == video_id, "Jellyfin missing YouTube provider id")
        require(provider_id(item, "primer-slug") == slug, "Jellyfin missing primer-slug provider id")
        if expected_plot:
            require((item.get("Overview") or "") == expected_plot, "Jellyfin overview does not match authored NFO plot")
        require(normalize_calendar_date(item.get("PremiereDate")) == expected_calendar,
                "Jellyfin premiere date calendar mismatch")
        ledger = json.loads((season.parent / ".primer-index.json").read_text())
        ep = ledger["episodes"][video_id]
        expected_key = f"S{ep['season']:02d}E{ep['episode']:03d}"
        require(record["episodeKey"] == expected_key, "TV/ledger episode key mismatch")
        require(record["title"] == f"{expected_show} {expected_key} — {expected_title}",
                "TV title does not match authored NFO title")
        require(normalize_calendar_date(record.get("uploadDate")) == expected_calendar,
                "TV upload-date calendar mismatch")
        archive = (season.parent / ".ytdlp-archive.txt").read_text().splitlines()
        require(archive.count("youtube " + video_id) == 1, "Missing/duplicate durable archive identity")
        stream = "/Videos/" + record["jellyfinItemId"] + "/stream"
        with jf.open(stream, {"static": "true"}, {"Range": "bytes=0-1048575"}) as response:
            require(response.status == 206, "Jellyfin must support byte-range streaming")
            chunk = response.read()
            with media.open("rb") as source:
                require(chunk == source.read(len(chunk)), "Stream range does not match the recovered/downloaded file")
            range_header = response.headers.get("Content-Range")
        with tempfile.TemporaryDirectory(prefix="primer-media-proof-") as work:
            downloaded = Path(work) / media.name
            with jf.open(stream, {"static": "true"}) as response, downloaded.open("wb") as dest:
                while chunk := response.read(1024 * 1024):
                    dest.write(chunk)
            sha = digest(media)
            require(digest(downloaded) == sha, "Jellyfin full stream differs from source media")
            seek = max(0, int(record["runtimeSeconds"]) // 2)
            result = subprocess.run(["ffmpeg", "-v", "error", "-ss", str(seek),
                                     "-i", str(downloaded), "-t", "5", "-map", "0:v:0", "-map", "0:a:0",
                                     "-progress", "pipe:1", "-nostats", "-f", "null", "-"],
                                    text=True, capture_output=True, timeout=120)
            frames = [int(x) for x in re.findall(r"^frame=(\d+)$", result.stdout, re.M)]
            require(result.returncode == 0 and frames and max(frames) > 0,
                    f"{video_id}: actual streamed audio/video failed decoding")
        evidence["videos"].append({"youtubeVideoId": video_id, "manifestSlug": slug,
            "mediaItemId": record["id"], "jellyfinItemId": item["Id"], "title": record["title"],
            "expectedTitle": expected_title, "expectedUploadDate": expected_calendar,
            "episodeKey": expected_key, "type": item["Type"], "runtimeSeconds": record["runtimeSeconds"],
            "bytes": media.stat().st_size, "sha256": sha, "range": range_header,
            "decodedFrames": max(frames), "decodedAudio": True, "seekSeconds": seek})
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(evidence, indent=2) + "\n")
    print(json.dumps(evidence, indent=2))


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        # No request objects, credentials, signed stream URLs, or subprocess argv.
        print(f"FAIL: {type(error).__name__}: {str(error)}", file=__import__("sys").stderr)
        raise SystemExit(1)
