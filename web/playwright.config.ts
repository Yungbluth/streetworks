import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "e2e",
  timeout: 30_000,
  use: { baseURL: "http://127.0.0.1:4173/" },
  webServer: [
    {
      command:
        "VITE_API=http://127.0.0.1:8091 npx vite build --outDir test-site --emptyOutDir && cp -r src/fixtures/data test-site/ && npx vite preview --outDir test-site --port 4173 --strictPort",
      url: "http://127.0.0.1:4173/",
      reuseExistingServer: false,
      timeout: 60_000,
    },
    {
      command: "go run ../cmd/streetworks serve -db src/fixtures/streetworks.db -addr 127.0.0.1:8091 -origin http://127.0.0.1:4173",
      url: "http://127.0.0.1:8091/healthz",
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});
