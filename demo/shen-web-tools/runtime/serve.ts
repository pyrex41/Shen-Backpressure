/**
 * runtime/serve.ts — Simple dev server for the Shen Web Tools demo.
 *
 * Serves:
 *   /              → index.html
 *   /static/*      → static assets (CSS)
 *   /src/*.shen    → Shen source files (loaded by the engine at runtime)
 *   /runtime/*.js  → Compiled TypeScript (from dist/ or transpiled on the fly)
 *
 * Usage: npx tsx runtime/serve.ts [port]
 */

import { createServer, IncomingMessage, ServerResponse } from "http";
import { readFileSync, existsSync } from "fs";
import { join, extname } from "path";
import { execSync } from "child_process";

const PORT = parseInt(process.argv[2] || "3000", 10);
const ROOT = join(import.meta.dirname || process.cwd(), "..");

const MIME: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "application/javascript; charset=utf-8",
  ".ts": "application/javascript; charset=utf-8",
  ".shen": "text/plain; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".map": "application/json; charset=utf-8",
};

// Try to build TypeScript first
try {
  console.log("Building TypeScript...");
  execSync("npx tsc 2>&1 || true", { cwd: ROOT, stdio: "pipe" });
} catch {
  console.warn("TypeScript build failed, will serve source files directly");
}

function serveFile(res: ServerResponse, filePath: string): boolean {
  if (!existsSync(filePath)) return false;
  const ext = extname(filePath);
  const mime = MIME[ext] || "application/octet-stream";
  let content = readFileSync(filePath, "utf-8");

  // For .ts files served as JS, do minimal transpilation via tsx
  // For runtime/*.js requests, check dist/ first, then try source
  res.writeHead(200, { "Content-Type": mime, "Access-Control-Allow-Origin": "*" });
  res.end(content);
  return true;
}

const server = createServer((req: IncomingMessage, res: ServerResponse) => {
  const url = req.url || "/";
  const path = url.split("?")[0];

  // Index
  if (path === "/" || path === "/index.html") {
    serveFile(res, join(ROOT, "index.html"));
    return;
  }

  // Shen source files
  if (path.startsWith("/src/") && path.endsWith(".shen")) {
    if (serveFile(res, join(ROOT, path.slice(1)))) return;
  }

  // Static assets
  if (path.startsWith("/static/")) {
    if (serveFile(res, join(ROOT, path.slice(1)))) return;
  }

  // Runtime JS — try dist/ first, then source .ts
  if (path.startsWith("/runtime/") && path.endsWith(".js")) {
    // Try compiled output
    const distPath = join(ROOT, "dist", path.slice(1));
    if (serveFile(res, distPath)) return;

    // Try .ts source (browser won't execute this, but useful for debugging)
    const tsPath = join(ROOT, path.slice(1).replace(".js", ".ts"));
    if (serveFile(res, tsPath)) return;
  }

  // 404
  res.writeHead(404, { "Content-Type": "text/plain" });
  res.end("Not found");
});

server.listen(PORT, () => {
  console.log(`\nShen Web Tools dev server`);
  console.log(`  http://localhost:${PORT}`);
  console.log(`\nArchitecture:`);
  console.log(`  Shen logic:  src/*.shen (loaded by engine at runtime)`);
  console.log(`  TS bridge:   runtime/bridge.ts (I/O dispatch)`);
  console.log(`  Arrow UI:    runtime/ui.ts (reactive rendering)`);
  console.log(`\nProvider config via URL params:`);
  console.log(`  ?search=mock|websearch`);
  console.log(`  ?fetch=mock|webfetch`);
  console.log(`  ?ai=mock|anthropic&key=YOUR_KEY`);
});
