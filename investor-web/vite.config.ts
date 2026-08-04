import { writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

const rootDir = path.dirname(fileURLToPath(import.meta.url));

/**
 * Emit robots.txt + sitemap URLs based on deploy env.
 * Staging/preview stay noindex; production requires VITE_ALLOW_INDEX=1 and
 * VITE_SITE_ENV=production.
 */
function seoArtifacts(): Plugin {
  return {
    name: "primer-seo-artifacts",
    closeBundle() {
      const outDir = path.resolve(rootDir, "dist");
      const origin = (process.env.VITE_SITE_ORIGIN ?? "").replace(/\/$/, "");
      const allowIndex =
        process.env.VITE_ALLOW_INDEX === "1" || process.env.VITE_ALLOW_INDEX === "true";
      const siteEnv = process.env.VITE_SITE_ENV ?? "";
      const isProdIndexable = allowIndex && siteEnv === "production";

      const sitemapLoc = origin ? `${origin}/sitemap.xml` : "/sitemap.xml";
      const robots = isProdIndexable
        ? [
            "# Production investor site",
            "User-agent: *",
            "Allow: /",
            "",
            `Sitemap: ${sitemapLoc}`,
            "",
          ].join("\n")
        : [
            "# Staging / preview — do not index",
            "User-agent: *",
            "Disallow: /",
            "",
            `Sitemap: ${sitemapLoc}`,
            "",
          ].join("\n");

      writeFileSync(path.join(outDir, "robots.txt"), robots);

      const paths = ["/", "/demo", "/market", "/evidence", "/schools", "/company", "/diligence"];
      const urls = paths
        .map((p) => {
          const loc = origin ? `${origin}${p === "/" ? "/" : p}` : p;
          return `  <url><loc>${loc}</loc></url>`;
        })
        .join("\n");
      const sitemap = [
        `<?xml version="1.0" encoding="UTF-8"?>`,
        `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`,
        urls,
        `</urlset>`,
        ``,
      ].join("\n");
      writeFileSync(path.join(outDir, "sitemap.xml"), sitemap);
    },
  };
}

export default defineConfig({
  plugins: [react(), seoArtifacts()],
  resolve: {
    alias: {
      "@": path.resolve(rootDir, "./src"),
    },
  },
  build: {
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: undefined,
      },
    },
  },
});
