import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const proxyTarget = process.env.DEV_API_PROXY_TARGET ?? "http://api:8080";
const allowedHosts = (process.env.DEV_ALLOWED_HOSTS ?? ".test,localhost,127.0.0.1")
  .split(",")
  .map((host) => host.trim())
  .filter(Boolean);
const hmrHost = process.env.HMR_HOST;
const strictCSP = "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; img-src 'self' data:; media-src 'self' data:; connect-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; script-src 'self'";
const devCSP = strictCSP.replace("script-src 'self'", "script-src 'self' 'unsafe-inline'");
const hmr = hmrHost
  ? {
      host: hmrHost,
      clientPort: Number(process.env.HMR_CLIENT_PORT ?? "80"),
      protocol: process.env.HMR_PROTOCOL ?? "ws",
    }
  : undefined;


export default defineConfig({
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
    headers: { "Content-Security-Policy": process.env.NODE_ENV === "production" ? strictCSP : devCSP },
    allowedHosts,
    hmr,
    proxy: {
      "/api": {
        target: proxyTarget,
        changeOrigin: true,
        secure: false,
        ws: true,
        configure: (proxy) => { proxy.on("proxyReq", (proxyReq, req) => { if (req.headers.host) proxyReq.setHeader("X-Forwarded-Host", req.headers.host); }); },
        rewrite: (path) => path.replace(/^\/api/, ""),
      },
      "/auth": {
        target: proxyTarget,
        changeOrigin: true,
        secure: false,
      },
      "/issuer": {
        target: "http://test-issuer:8091",
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
