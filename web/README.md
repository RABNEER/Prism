# PRISM evidence desk

Next.js/React viewer backed by the actual Go MCP through a local Python bridge.

See [the focused demo guide](../docs/PUNE_DEMO.md) for prerequisites, Firecrawl configuration, checks and source limits.

After building the repository binaries, run `npm run dev -- --hostname 127.0.0.1` here and open http://127.0.0.1:3000. Run `npm run lint` and `npm run build` for frontend checks.

This is a local demo. The repository, Python, Go MCP binary and Rust matching binary must remain available to the server process. Synthetic examples use a separate store.
