# Effects

Use this only when applying native Figma effects through `set_effects` or creating
effect styles with `create_effect_style`. For general design judgment, prefer the
design-system effect style guidance in `figma-design-patterns`.

**Quick navigation:** [Effect types](#effect-types) · [Example](#example) · [Notes](#notes)

---

## Effect types

Allowed `type` values:

```
DROP_SHADOW | INNER_SHADOW | LAYER_BLUR | BACKGROUND_BLUR | GLASS | NOISE | TEXTURE
```

`GLASS`, `NOISE`, and `TEXTURE` are Figma native 2025 effects. Fields are optional
and defaulted; pass only overrides.

| Type | Fields |
|---|---|
| `DROP_SHADOW` / `INNER_SHADOW` | `color`, `opacity`, `offsetX`, `offsetY`, `radius`, `spread`, `showShadowBehindNode` for drop shadow |
| `LAYER_BLUR` / `BACKGROUND_BLUR` | `radius` |
| `GLASS` | `lightIntensity`, `lightAngle`, `refraction`, `depth`, `dispersion`, frost `radius` |
| `TEXTURE` | `noiseSize`, `noiseSizeVector`, `radius`, `clipToShape` |
| `NOISE` | `noiseType`, `color`, `secondaryColor`, `opacity`, `noiseSize`, `noiseSizeVector`, `density` |

## Example

```jsonc
{
  "type": "set_effects",
  "nodeIds": ["<id>"],
  "params": {
    "effects": [
      { "type": "GLASS", "lightIntensity": 0.6, "refraction": 0.35, "depth": 8, "dispersion": 0.12, "radius": 14 }
    ]
  }
}
```

## Notes

- `set_effects` replaces all effects on the node. Pass `[]` to clear.
- `GLASS` refracts layers behind it, so it reads only over a transparent or semi-transparent fill with content behind it.
- Older plugin builds reject `GLASS`, `NOISE`, and `TEXTURE` with a validation error that lists only classic effect types.
