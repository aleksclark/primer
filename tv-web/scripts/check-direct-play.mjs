// Lightweight guard for direct-play audio allowlist (no test runner in package).
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const src = readFileSync(join(root, "src/lib/directPlay.ts"), "utf8");

const required = ["aac", "mp3", "ac3", "eac3", "dts", "opus", "vorbis", "flac"];
for (const codec of required) {
  if (!src.includes(`"${codec}"`)) {
    console.error(`directPlay.ts missing audio codec "${codec}"`);
    process.exit(1);
  }
}
// Ensure truehd is not silently allowlisted (CPU / not enabled in FFmpeg build).
if (src.includes('"truehd"')) {
  console.error("truehd must not be in DIRECT_PLAY_AUDIO");
  process.exit(1);
}
console.log("direct-play allowlist OK:", required.join(", "));
