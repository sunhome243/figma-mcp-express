# MCP Known Bugs — figma-mcp-express server issues

Server bugs confirmed in production, with workarounds. Each links a GitHub issue.
Check the running server version (`figma-mcp-express --version`) — a fix on `main` only
takes effect once it ships in a tagged release.

## `$`+digit in batch text parsed as an op-ref (issue #82)

Inside `batch(ops:[...])`, a text value where `$` is immediately followed by digits — e.g.
`set_text`/`create_text` with `"$12.2B"` — collides with the op-ref syntax (`$N` = "result of
op N"): `$12` resolves as "ref points to op #12" and the build fails. Common in finance / IR /
pricing copy. A leading lone `$` (no trailing digit) is safe.

**Workaround (revise-when-fixed):** insert a zero-width space (U+200B) between `$` and the
first digit so the adjacency breaks — but this injects an invisible char into the deliverable
copy, so prefer a non-`batch` `set_text` for dollar-heavy text until the fix ships. The fix
(issue #82) makes `$N` resolution skip quoted string values.

---

## Version note — fill variable bindings on reads (issue #27)

Reads now surface fill color-variable bindings: a bound `SOLID` fill serializes as
`{color, variableId}` instead of a bare hex, so a bound token and a raw hex are no longer
byte-identical and D3 token-binding can be verified directly. **This ships in the next release.**
On `2.3.0` and earlier, fills flatten to hex with no binding — fall back to the off-palette-hex
heuristic (a hex not in the project token spine is a raw-fill violation; palette-matching values
are presumed bound). See `references/gotchas.md`.
