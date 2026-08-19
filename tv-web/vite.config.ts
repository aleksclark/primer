import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const apiProxyTarget =
  process.env.VITE_API_PROXY_TARGET ?? "http://localhost:8081";
const devHost = process.env.VITE_DEV_HOST ?? "127.0.0.1";
const devPort = Number(process.env.VITE_DEV_PORT ?? "5174");
const hmrHost = process.env.VITE_HMR_HOST?.trim() || undefined;
const hmrClientPort = process.env.VITE_HMR_CLIENT_PORT
  ? Number(process.env.VITE_HMR_CLIENT_PORT)
  : undefined;
const hmrProtocol = process.env.VITE_HMR_PROTOCOL?.trim() || undefined;
const allowedHosts = (process.env.VITE_ALLOWED_HOSTS ?? ".test,localhost,127.0.0.1")
  .split(",")
  .map((h) => h.trim())
  .filter(Boolean);

const hmr =
  hmrHost != null
    ? {
        host: hmrHost,
        clientPort: hmrClientPort ?? 3001,
        protocol: (hmrProtocol ?? "ws") as "ws" | "wss",
        port: Number.isFinite(devPort) ? devPort : 5174,
      }
    : undefined;

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    host: devHost,
    port: Number.isFinite(devPort) ? devPort : 5174,
    strictPort: true,
    allowedHosts,
    ...(hmr ? { hmr } : {}),
    proxy: {
      "/api": {
        target: apiProxyTarget,
        changeOrigin: true,
      },
    },
  },
});
