---
name: figma-design-md
description: Use when extracting DESIGN.md from a Figma file, including tokens, styles, components, rationale, and optional Tailwind or DTCG exports.
---

# Figma DESIGN.md Extraction

Read-only workflow for producing a `DESIGN.md` from a Figma file. The Figma tools are the source of truth; screenshots only provide aesthetic prose signals.

## Input

`/figma-design-md <figma-file-url-or-filekey>`

Extract `fileKey` from `figma.com/design/<fileKey>/...`. Default output is `./DESIGN.md`; honor `--output <path>` when present.

## Workflow

1. Bootstrap in parallel: `list_channels`, `npx --yes @google/design.md spec --format json`, and output-path resolution.
2. Harvest in parallel: `export_tokens`, `get_styles`, `get_local_components`, and `get_pages`.
3. Pick up to three representative frames from non-archive pages, then read each with `get_design_context detail:"full"` and `save_screenshots`.
4. Compose `<output>.new` using tool data as factual truth. Never invent token values or override tool data with screenshot impressions.
5. Run `npx --yes @google/design.md lint <output>.new --format json`; regenerate at most three times before asking the user.
6. Promote atomically and optionally export Tailwind/DTCG with `@google/design.md export`.

## References

- `references/token-extraction-spec.md` defines exactly what each Figma tool contributes and how gaps are handled.
- `references/design-md-schema.md` defines YAML front matter and token value types.
- `references/design-md-sections.md` defines body section order, prose requirements, and component-token guidance.

## Delivery

Report the output path, selected source frames, token/component counts, lint findings, and export files. If a token category is absent, say it was absent rather than fabricating placeholders.
