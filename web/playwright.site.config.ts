import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "e2e-site",
  timeout: 60_000,
  use: { baseURL: "http://127.0.0.1:4174/" },
  webServer: {
    command: "npx vite preview --outDir ../site --port 4174 --strictPort",
    url: "http://127.0.0.1:4174/",
    reuseExistingServer: false,
    timeout: 60_000,
  },
});
