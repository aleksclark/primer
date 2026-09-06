import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const proxyTarget = process.env.DEV_API_PROXY_TARGET ?? "http://api:8080";
const allowedHosts = (process.env.DEV_ALLOWED_HOSTS ?? ".test,localhost,127.0.0.1")
  .split(",")
  .map((host) => host.trim())
  .filter(Boolean);
const hmrHost = process.env.HMR_HOST;
const hmr = hmrHost
  ? {
      host: hmrHost,
      clientPort: Number(process.env.HMR_CLIENT_PORT ?? "80"),
      protocol: process.env.HMR_PROTOCOL ?? "ws",
    }
  : undefined;

const base = process.env.VITE_TASKS_BASE_PATH ?? "/";
if (base !== "/" && base !== "/tasks/") throw new Error("VITE_TASKS_BASE_PATH must be / or /tasks/");
if (base === "/tasks/" && !process.env.VITE_CLERK_PUBLISHABLE_KEY) {
  throw new Error("VITE_CLERK_PUBLISHABLE_KEY is required for the release bundle");
}
if (base === "/tasks/" && process.env.VITE_TASKS_API_BASE !== "/tasks/api") {
  throw new Error("VITE_TASKS_API_BASE must be /tasks/api for the release bundle");
}

export default defineConfig({
  base,
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    host: process.env.DEV_SERVER_HOST ?? "0.0.0.0",
    port: Number(process.env.DEV_SERVER_PORT ?? "5173"),
    strictPort: true,
    allowedHosts,
    hmr,
    proxy: {
      "/api": {
        target: proxyTarget,
        ws: true,
        changeOrigin: true,
        secure: false,
        configure: (proxy) => {
          proxy.on("proxyReq", (proxyReq, req) => {
            if (req.headers.host) proxyReq.setHeader("X-Forwarded-Host", req.headers.host);
          });
        },
        rewrite: (path) => path.replace(/^\/api/, ""),
      },
      "/auth": {
        target: proxyTarget,
        changeOrigin: true,
        secure: false,
        configure: (proxy) => {
          proxy.on("proxyReq", (proxyReq, req) => {
            if (req.headers.host) proxyReq.setHeader("X-Forwarded-Host", req.headers.host);
          });
        },
      },
      "/issuer": {
        target: process.env.DEV_ISSUER_PROXY_TARGET ?? "http://test-issuer:8091",
        changeOrigin: true,
        secure: false,
        rewrite: (path) => path.replace(/^\/issuer/, ""),
      },
      "/ws": {
        target: proxyTarget,
        changeOrigin: true,
        secure: false,
        ws: true,
      },
    },
  },
});
