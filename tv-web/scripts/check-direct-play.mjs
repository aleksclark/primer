// Guard: tv-web direct-play allowlists stay aligned with the Android client
// capability and the server-side policy in server/internal/tv/directplay.
//
// Run via `npm run check:direct-play` (wired into prebuild).
import { readFileSync, existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = join(root, "..");

const REQUIRED_VIDEO = ["h264", "hevc", "h265", "mpeg4", "vp8", "vp9"];
const REQUIRED_AUDIO = ["aac", "mp3", "ac3", "eac3", "dts", "opus", "vorbis", "flac"];
const FORBIDDEN_AUDIO = ["truehd", "dtshd", "mlp", "pcm"];
const FORBIDDEN_VIDEO = ["av1", "vc1", "mpeg2video"];

function fail(msg) {
  console.error(msg);
  process.exitCode = 1;
}

function extractConstArray(src, name) {
  const re = new RegExp(
    `export\\s+const\\s+${name}\\s*=\\s*\\[([\\s\\S]*?)\\]\\s*as\\s+const`,
  );
  const m = src.match(re);
  if (!m) {
    fail(`directPlay.ts missing export const ${name} = [...] as const`);
    return [];
  }
  return [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
}

function extractGoStringSlice(src, name) {
  const re = new RegExp(`${name}\\s*=\\s*\\[\\]string\\{([^}]*)\\}`, "m");
  const m = src.match(re);
  if (!m) {
    fail(`server policy missing ${name} = []string{...}`);
    return [];
  }
  return [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
}

function sameList(label, actual, expected) {
  const a = [...actual].sort().join(",");
  const e = [...expected].sort().join(",");
  if (a !== e) {
    fail(`${label} mismatch\n  actual:   [${actual.join(", ")}]\n  expected: [${expected.join(", ")}]`);
  }
}

// --- tv-web allowlist ---
const tsPath = join(root, "src/lib/directPlay.ts");
const tsSrc = readFileSync(tsPath, "utf8");
const tsVideo = extractConstArray(tsSrc, "DIRECT_PLAY_VIDEO");
const tsAudio = extractConstArray(tsSrc, "DIRECT_PLAY_AUDIO");

sameList("DIRECT_PLAY_VIDEO", tsVideo, REQUIRED_VIDEO);
sameList("DIRECT_PLAY_AUDIO", tsAudio, REQUIRED_AUDIO);

for (const codec of FORBIDDEN_AUDIO) {
  if (tsAudio.includes(codec)) {
    fail(`DIRECT_PLAY_AUDIO must not include forbidden codec "${codec}"`);
  }
}
for (const codec of FORBIDDEN_VIDEO) {
  if (tsVideo.includes(codec)) {
    fail(`DIRECT_PLAY_VIDEO must not include forbidden codec "${codec}"`);
  }
}

// Ensure directPlayIssues still exists (import surface for library.tsx).
if (!tsSrc.includes("export function directPlayIssues")) {
  fail("directPlay.ts missing export function directPlayIssues");
}

// --- server allowlist (when present in monorepo checkout) ---
const goPath = join(repoRoot, "server/internal/tv/directplay/policy.go");
if (!existsSync(goPath)) {
  console.warn("skip server allowlist check: policy.go not found at", goPath);
} else {
  const goSrc = readFileSync(goPath, "utf8");
  const goVideo = extractGoStringSlice(goSrc, "VideoAllowlist");
  const goAudio = extractGoStringSlice(goSrc, "AudioAllowlist");
  sameList("server VideoAllowlist vs DIRECT_PLAY_VIDEO", goVideo, tsVideo);
  sameList("server AudioAllowlist vs DIRECT_PLAY_AUDIO", goAudio, tsAudio);
}

if (process.exitCode) {
  console.error("direct-play allowlist check FAILED");
  process.exit(process.exitCode);
}

console.log(
  "direct-play allowlist OK:",
  `video=[${REQUIRED_VIDEO.join(", ")}]`,
  `audio=[${REQUIRED_AUDIO.join(", ")}]`,
  existsSync(goPath) ? "(server+ts matched)" : "(ts only)",
);
