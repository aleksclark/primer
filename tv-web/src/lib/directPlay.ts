/**
 * Codecs the Android client can direct-play without Jellyfin transcoding.
 * Video is SoC hardware; AC3/EAC3/DTS use the app's bundled Media3 FFmpeg
 * software fallback (LGPL). Keep Jellyfin codec names lowercase as reported
 * by the browse API (e.g. "eac3", "dts").
 */
export const DIRECT_PLAY_VIDEO = ["h264", "hevc", "h265", "mpeg4", "vp8", "vp9"] as const;

export const DIRECT_PLAY_AUDIO = [
  "aac",
  "mp3",
  "ac3",
  "eac3",
  "dts",
  "opus",
  "vorbis",
  "flac",
] as const;

export type BrowseCodecs = {
  videoCodec?: string | null;
  audioCodec?: string | null;
};

/** Reasons an item would need transcoding (empty => direct-play OK). */
export function directPlayIssues(item: BrowseCodecs): string[] {
  const issues: string[] = [];
  const video = item.videoCodec?.toLowerCase() ?? "";
  const audio = item.audioCodec?.toLowerCase() ?? "";
  if (video && !(DIRECT_PLAY_VIDEO as readonly string[]).includes(video)) {
    issues.push(`video codec ${item.videoCodec}`);
  }
  if (audio && !(DIRECT_PLAY_AUDIO as readonly string[]).includes(audio)) {
    issues.push(`audio codec ${item.audioCodec}`);
  }
  return issues;
}
