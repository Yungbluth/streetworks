import { createReadStream, existsSync, statSync } from "node:fs";
import { join, normalize, resolve } from "node:path";
import { defineConfig, type Plugin } from "vitest/config";

const site = resolve(__dirname, "../site");

// in development the data comes straight from ../site/data, where `streetworks build` writes it
function serveData(): Plugin {
  return {
    name: "serve-site-data",
    configureServer(server) {
      server.middlewares.use("/data", (req, res, next) => {
        const path = normalize(join(site, "data", decodeURIComponent((req.url ?? "/").split("?")[0]!)));
        if (!path.startsWith(join(site, "data")) || !existsSync(path) || !statSync(path).isFile()) return next();
        res.setHeader("Content-Type", "application/json");
        createReadStream(path).pipe(res);
      });
    },
  };
}

export default defineConfig({
  // Relative URLs, so the site works under a GitHub Pages project path
  base: "./",
  plugins: [serveData()],
  build: {
    outDir: site,
    emptyOutDir: false,
  },
  test: {
    include: ["src/**/*.test.ts"],
  },
});
