import { spawn } from "node:child_process";
import path from "node:path";
export const runtime = "nodejs";
export const maxDuration = 240;
export async function POST(request: Request) {
  // Next's internal request URL may use localhost even when the browser uses 127.0.0.1.
  const host = request.headers.get("host") || new URL(request.url).host;
  let externalOrigin: URL;
  try { externalOrigin = new URL(`http://${host}`); } catch { return Response.json({ ok: false, error: "Invalid host" }, { status: 403 }); }
  if (!["127.0.0.1", "localhost"].includes(externalOrigin.hostname)) return Response.json({ ok: false, error: "Local demo only" }, { status: 403 });
  const origin = request.headers.get("origin");
  if (origin && origin !== externalOrigin.origin) return Response.json({ ok: false, error: "Cross-origin request rejected" }, { status: 403 });
  const text = await request.text();
  if (Buffer.byteLength(text) > 32768) return Response.json({ ok: false, error: "Request too large" }, { status: 413 });
  let payload: Record<string, unknown>;
  try { payload = JSON.parse(text); } catch { return Response.json({ ok: false, error: "Invalid JSON" }, { status: 400 }); }
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) return Response.json({ ok: false, error: "JSON object required" }, { status: 400 });
  if (!["bootstrap", "investigate", "ingest", "example", "evidence", "registry_sample", "discover", "ingest_discovered"].includes(String(payload.action))) return Response.json({ ok: false, error: "Unsupported action" }, { status: 400 });
  const repo = process.env.PRISM_REPO_DIR || path.resolve(process.cwd(), "..");
  const output = await new Promise<{ ok: boolean; data?: unknown; error?: string }>((resolve) => {
    // The bridge and compiled binaries are supplied by this local repository, not bundled by Next.
    const child = spawn(/* turbopackIgnore: true */ process.env.PRISM_PYTHON || "python", ["-X", "utf8", path.join(repo, "scripts", "demo_bridge.py")], { cwd: repo, env: process.env, windowsHide: true, stdio: ["pipe", "pipe", "pipe"] });
    let stdout = ""; let settled = false;
    const finish = (value: { ok: boolean; data?: unknown; error?: string }) => { if (!settled) { settled = true; clearTimeout(timer); resolve(value); } };
    const timer = setTimeout(() => { child.kill(); finish({ ok: false, error: "Source request timed out. Retry with cached evidence or disable location retrieval." }); }, 230000);
    child.stdout.on("data", (chunk) => { stdout += chunk.toString("utf8"); if (stdout.length > 12000000) { child.kill(); finish({ ok: false, error: "Response too large" }); } });
    child.stderr.on("data", () => {});
    child.on("error", () => finish({ ok: false, error: "Could not start the MCP bridge. Build the binaries and check Python." }));
    child.on("close", () => { try { finish(JSON.parse(stdout)); } catch { finish({ ok: false, error: "MCP bridge returned an invalid response" }); } });
    child.stdin.on("error", () => {}); child.stdin.end(JSON.stringify(payload));
  });
  return Response.json(output, { status: output.ok ? 200 : 422, headers: { "Cache-Control": "no-store" } });
}
