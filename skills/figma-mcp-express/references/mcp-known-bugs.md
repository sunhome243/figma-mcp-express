# MCP Known Bugs — figma-mcp-express server issues

Server bugs confirmed in production, with workarounds. Each links a GitHub issue.
Check the running server version (`figma-mcp-express --version`) — a fix on `main` only
takes effect once it ships in a tagged release.

No open server bugs are currently tracked.

---

## Version note — fill variable bindings on reads (issue #27)

Reads now surface fill color-variable bindings: a bound `SOLID` fill serializes as
`{color, variableId}` instead of a bare hex, so a bound token and a raw hex are no longer
byte-identical and D3 token-binding can be verified directly. **This ships in the next release.**
On `2.3.0` and earlier, fills flatten to hex with no binding — fall back to the off-palette-hex
heuristic (a hex not in the project token spine is a raw-fill violation; palette-matching values
are presumed bound). See `references/gotchas.md`.
