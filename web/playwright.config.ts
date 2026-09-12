import { defineConfig } from "@playwright/test";

// the smoke test runs against a build of the page with the fixture data
export default defineConfig({
  testDir: "e2e",
  timeout: 30_000,
  use: { baseURL: "http://127.0.0.1:4173/" },
  webServer: {
    command: "npx vite build --outDir test-site --emptyOutDir && cp -r src/fixtures/data test-site/ && npx vite preview --outDir test-site --port 4173 --strictPort",
    url: "http://127.0.0.1:4173/",
    reuseExistingServer: false,
    timeout: 60_000,
  },
});
