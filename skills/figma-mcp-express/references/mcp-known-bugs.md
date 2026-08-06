# MCP Known Bugs — figma-mcp-express server issues

Server bugs confirmed in production, with workarounds. Each links a GitHub issue.
Check the running server version (`figma-mcp-express --version`) — a fix on `main` only
takes effect once it ships in a tagged release.

## Standalone currency text can match a batch op-ref (issue #82)

Inside `batch(ops:[...])`, a string whose entire value matches `$N.path` is treated as an
op-ref (`N` = an earlier op index). Standalone finance / IR / pricing copy such as `"$12.2B"`
therefore resolves as op 12 plus field `2B` and fails. Embedded copy such as
`"Revenue: $12.2B"` and a value without a dotted suffix such as `"$12"` do not match the
anchored op-ref grammar and remain literal.

**Workaround (revise-when-fixed):** in the default `core` profile, insert a zero-width space
(U+200B) between `$` and the first digit, then verify the delivered copy because this adds an
invisible character. If `FIGMA_MCP_TOOL_PROFILE=full` is already enabled, use the non-`batch`
`set_text` tool instead. Remove this workaround after issue #82 ships in a tagged release.

---

## Version note — fill variable bindings on reads (issue #27)

Reads now surface fill color-variable bindings: a bound `SOLID` fill serializes as
`{color, variableId}` instead of a bare hex, so a bound token and a raw hex are no longer
byte-identical and D3 token-binding can be verified directly. **This ships in the next release.**
On `2.3.0` and earlier, fills flatten to hex with no binding — fall back to the off-palette-hex
heuristic (a hex not in the project token spine is a raw-fill violation; palette-matching values
are presumed bound). See `references/gotchas.md`.
