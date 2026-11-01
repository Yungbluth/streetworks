import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "e2e-site",
  timeout: 60_000,
  use: { baseURL: "http://127.0.0.1:4174/" },
  webServer: [
    {
      command: "npx vite preview --outDir ../site --port 4174 --strictPort",
      url: "http://127.0.0.1:4174/",
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command: "go run ../cmd/streetworks serve -db ../streetworks.db -addr 127.0.0.1:8092 -origin http://127.0.0.1:4174",
      url: "http://127.0.0.1:8092/health",
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});
