import { defineConfig } from "vite";

export default defineConfig({
  server: {
    host: "127.0.0.1",
    proxy: {
      "/v1": "http://127.0.0.1:8787",
      "/healthz": "http://127.0.0.1:8787",
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
