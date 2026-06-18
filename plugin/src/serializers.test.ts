import { describe, it, expect, beforeEach } from "bun:test";
import {
  isMixed,
  toHex,
  serializePaints,
  getBounds,
  deduplicateStyles,
  serializeVariableValue,
  serializeLineHeight,
  serializeLetterSpacing,
  serializeStyles,
  serializeText,
  serializeNode,
} from "./serializers";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockGetStyleByIdAsync: (id: string) => Promise<{ name: string } | null>;

beforeEach(() => {
  mockGetStyleByIdAsync = async (_id: string) => null;
  (globalThis as any).figma = {
    getStyleByIdAsync: (id: string) => mockGetStyleByIdAsync(id),
  };
});

// ── isMixed ──────────────────────────────────────────────────────────────────

describe("isMixed", () => {
  it("returns true for symbols", () => {
    expect(isMixed(Symbol())).toBe(true);
  });
  it("returns false for non-symbols", () => {
    expect(isMixed(14)).toBe(false);
    expect(isMixed("hello")).toBe(false);
    expect(isMixed(null)).toBe(false);
    expect(isMixed(undefined)).toBe(false);
  });
});

// ── toHex ────────────────────────────────────────────────────────────────────

describe("toHex", () => {
  it("converts full white", () => {
    expect(toHex({ r: 1, g: 1, b: 1 })).toBe("#ffffff");
  });
  it("converts full black", () => {
    expect(toHex({ r: 0, g: 0, b: 0 })).toBe("#000000");
  });
  it("converts a mid-range color", () => {
    expect(toHex({ r: 1, g: 0, b: 0 })).toBe("#ff0000");
  });
  it("clamps values above 1", () => {
    expect(toHex({ r: 2, g: 0, b: 0 })).toBe("#ff0000");
  });
  it("clamps values below 0", () => {
    expect(toHex({ r: -1, g: 0, b: 0 })).toBe("#000000");
  });
  it("rounds fractional values", () => {
    // 0.5 * 255 = 127.5 → rounds to 128 = 0x80
    expect(toHex({ r: 0.5, g: 0.5, b: 0.5 })).toBe("#808080");
  });
});

// ── serializePaints ───────────────────────────────────────────────────────────

describe("serializePaints", () => {
  it("returns 'mixed' for symbol input", () => {
    expect(serializePaints(Symbol())).toBe("mixed");
  });
  it("returns undefined for null/non-array", () => {
    expect(serializePaints(null)).toBeUndefined();
    expect(serializePaints("red")).toBeUndefined();
  });
  it("returns undefined for empty array", () => {
    expect(serializePaints([])).toBeUndefined();
  });
  it("emits {type} objects for non-SOLID paints — never silently drops IMAGE/GRADIENT", () => {
    // Pre-fix: IMAGE/GRADIENT were silently dropped → undefined (the bug).
    // Post-fix: non-SOLID paints are emitted as {type} objects so consumers know they exist.
    const paints = [{ type: "IMAGE" }, { type: "GRADIENT_LINEAR" }];
    expect(serializePaints(paints)).toEqual([{ type: "IMAGE" }, { type: "GRADIENT_LINEAR" }]);
  });
  it("emits IMAGE paint with scaleMode and imageHash when present", () => {
    const paints = [{ type: "IMAGE", scaleMode: "FILL", imageHash: "abc123" }];
    expect(serializePaints(paints)).toEqual([{ type: "IMAGE", scaleMode: "FILL", imageHash: "abc123" }]);
  });
  it("emits IMAGE paint without scaleMode/imageHash when absent", () => {
    const paints = [{ type: "IMAGE" }];
    expect(serializePaints(paints)).toEqual([{ type: "IMAGE" }]);
  });
  it("mixed array of SOLID + IMAGE returns both (solid as hex string, image as object)", () => {
    const paints = [
      { type: "SOLID", color: { r: 1, g: 0, b: 0 }, opacity: 1 },
      { type: "IMAGE", scaleMode: "FILL", imageHash: "xyz" },
    ];
    expect(serializePaints(paints)).toEqual(["#ff0000", { type: "IMAGE", scaleMode: "FILL", imageHash: "xyz" }]);
  });
  it("serializes a solid paint with opacity 1 as plain hex", () => {
    const paints = [{ type: "SOLID", color: { r: 1, g: 0, b: 0 }, opacity: 1 }];
    expect(serializePaints(paints)).toEqual(["#ff0000"]);
  });
  it("appends alpha hex when opacity < 1", () => {
    // opacity 0.5 → Math.round(0.5 * 255) = 128 = 0x80
    const paints = [{ type: "SOLID", color: { r: 1, g: 0, b: 0 }, opacity: 0.5 }];
    const result = serializePaints(paints) as string[];
    expect(result[0]).toBe("#ff000080");
  });
  it("defaults opacity to 1 when not provided", () => {
    const paints = [{ type: "SOLID", color: { r: 0, g: 0, b: 1 } }];
    expect(serializePaints(paints)).toEqual(["#0000ff"]);
  });
  it("serializes multiple solid paints", () => {
    const paints = [
      { type: "SOLID", color: { r: 1, g: 0, b: 0 } },
      { type: "SOLID", color: { r: 0, g: 1, b: 0 } },
    ];
    expect(serializePaints(paints)).toEqual(["#ff0000", "#00ff00"]);
  });
  it("surfaces a color variable binding as {color, variableId} (issue #27)", () => {
    const paints = [
      {
        type: "SOLID",
        color: { r: 0, g: 0, b: 0 },
        opacity: 1,
        boundVariables: { color: { type: "VARIABLE_ALIAS", id: "VariableID:1:2" } },
      },
    ];
    expect(serializePaints(paints)).toEqual([
      { color: "#000000", variableId: "VariableID:1:2" },
    ]);
  });
  it("keeps an unbound fill as a bare hex string", () => {
    const paints = [{ type: "SOLID", color: { r: 0, g: 0, b: 0 }, opacity: 1 }];
    expect(serializePaints(paints)).toEqual(["#000000"]);
  });
});

// ── getBounds ─────────────────────────────────────────────────────────────────

describe("getBounds", () => {
  it("returns bounds for a node with x/y/width/height", () => {
    expect(getBounds({ x: 10, y: 20, width: 100, height: 50 })).toEqual({
      x: 10, y: 20, width: 100, height: 50,
    });
  });
  it("rounds floating point values to 2 decimal places", () => {
    const bounds = getBounds({ x: 10.999, y: 0, width: 99.999, height: 50 });
    expect(bounds?.x).toBe(11);
    expect(bounds?.width).toBe(100);
  });
  it("returns undefined when coordinates are missing", () => {
    expect(getBounds({ name: "page" })).toBeUndefined();
    expect(getBounds({ x: 0, y: 0 })).toBeUndefined();
  });
});

// ── serializeLineHeight ───────────────────────────────────────────────────────

describe("serializeLineHeight", () => {
  it("returns 'mixed' for symbol", () => {
    expect(serializeLineHeight(Symbol())).toBe("mixed");
  });
  it("returns undefined for AUTO unit", () => {
    expect(serializeLineHeight({ unit: "AUTO" })).toBeUndefined();
  });
  it("returns undefined for null/falsy", () => {
    expect(serializeLineHeight(null)).toBeUndefined();
    expect(serializeLineHeight(undefined)).toBeUndefined();
  });
  it("returns value and unit for PIXELS", () => {
    expect(serializeLineHeight({ value: 24, unit: "PIXELS" })).toEqual({ value: 24, unit: "PIXELS" });
  });
  it("returns value and unit for PERCENT", () => {
    expect(serializeLineHeight({ value: 150, unit: "PERCENT" })).toEqual({ value: 150, unit: "PERCENT" });
  });
});

// ── serializeLetterSpacing ────────────────────────────────────────────────────

describe("serializeLetterSpacing", () => {
  it("returns 'mixed' for symbol", () => {
    expect(serializeLetterSpacing(Symbol())).toBe("mixed");
  });
  it("returns undefined when value is 0", () => {
    expect(serializeLetterSpacing({ value: 0, unit: "PIXELS" })).toBeUndefined();
  });
  it("returns undefined for null/falsy", () => {
    expect(serializeLetterSpacing(null)).toBeUndefined();
  });
  it("returns value and unit for non-zero spacing", () => {
    expect(serializeLetterSpacing({ value: 1.5, unit: "PIXELS" })).toEqual({ value: 1.5, unit: "PIXELS" });
  });
});

// ── deduplicateStyles ─────────────────────────────────────────────────────────

describe("deduplicateStyles", () => {
  it("returns original tree and undefined globalVars when nothing is repeated", () => {
    const tree = {
      children: [
        { styles: { fills: ["#ff0000"] } },
        { styles: { fills: ["#00ff00"] } },
      ],
    };
    const { tree: result, globalVars } = deduplicateStyles(tree);
    expect(globalVars).toBeUndefined();
    expect(result).toBe(tree);
  });

  it("deduplicates fills that appear more than once", () => {
    const sharedFill = ["#ff0000"];
    const tree = {
      children: [
        { styles: { fills: sharedFill } },
        { styles: { fills: sharedFill } },
      ],
    };
    const { tree: result, globalVars } = deduplicateStyles(tree);
    expect(globalVars).toBeDefined();
    const refs = Object.keys(globalVars!.styles);
    expect(refs.length).toBe(1);
    // Both nodes should now reference the short key instead of the array
    const children = (result as any).children;
    expect(typeof children[0].styles.fills).toBe("string");
    expect(children[0].styles.fills).toBe(children[1].styles.fills);
  });

  it("deduplicates strokes that appear more than once", () => {
    const sharedStroke = ["#0000ff"];
    const tree = {
      children: [
        { styles: { strokes: sharedStroke } },
        { styles: { strokes: sharedStroke } },
      ],
    };
    const { globalVars } = deduplicateStyles(tree);
    expect(globalVars).toBeDefined();
  });

  it("preserves unique fills as-is", () => {
    const tree = {
      children: [
        { styles: { fills: ["#ff0000"] } },
        { styles: { fills: ["#00ff00"] } },
        { styles: { fills: ["#ff0000"] } },
        { styles: { fills: ["#00ff00"] } },
      ],
    };
    const { globalVars } = deduplicateStyles(tree);
    // Both colors appear twice so both should be deduped
    expect(Object.keys(globalVars!.styles).length).toBe(2);
  });

  it("handles empty tree without errors", () => {
    const { tree, globalVars } = deduplicateStyles({});
    expect(globalVars).toBeUndefined();
    expect(tree).toEqual({});
  });
});

// ── serializeVariableValue ────────────────────────────────────────────────────

describe("serializeVariableValue", () => {
  it("passes through primitives unchanged", () => {
    expect(serializeVariableValue(42)).toBe(42);
    expect(serializeVariableValue("hello")).toBe("hello");
    expect(serializeVariableValue(true)).toBe(true);
    expect(serializeVariableValue(null)).toBe(null);
  });

  it("serializes VARIABLE_ALIAS objects", () => {
    const val = { type: "VARIABLE_ALIAS", id: "abc123", extra: "ignored" };
    expect(serializeVariableValue(val)).toEqual({ type: "VARIABLE_ALIAS", id: "abc123" });
  });

  it("serializes color objects to COLOR type", () => {
    const val = { r: 1, g: 0, b: 0, a: 1 };
    expect(serializeVariableValue(val)).toEqual({ type: "COLOR", r: 1, g: 0, b: 0, a: 1 });
  });

  it("defaults alpha to 1 when missing from color", () => {
    const val = { r: 0, g: 1, b: 0 };
    expect(serializeVariableValue(val)).toEqual({ type: "COLOR", r: 0, g: 1, b: 0, a: 1 });
  });

  it("passes through unknown objects unchanged", () => {
    const val = { foo: "bar" };
    expect(serializeVariableValue(val)).toEqual({ foo: "bar" });
  });
});

// ── serializeStyles ───────────────────────────────────────────────────────────

describe("serializeStyles", () => {
  it("returns empty object for node with no relevant properties", async () => {
    const result = await serializeStyles({ id: "1", name: "box" });
    expect(result).toEqual({});
  });

  it("includes fills when fills is a solid paint array", async () => {
    const node = { fills: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 } }] };
    const result = await serializeStyles(node);
    expect(result.fills).toEqual(["#ff0000"]);
  });

  it("includes fillStyle name when fillStyleId resolves to a style", async () => {
    mockGetStyleByIdAsync = async (id) => (id === "style-1" ? { name: "Red" } : null);
    const node = {
      fills: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 } }],
      fillStyleId: "style-1",
    };
    const result = await serializeStyles(node);
    expect(result.fillStyle).toBe("Red");
    expect(result.fills).toEqual(["#ff0000"]);
  });

  it("skips fillStyle when fillStyleId resolves to null", async () => {
    const node = {
      fills: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 } }],
      fillStyleId: "missing",
    };
    const result = await serializeStyles(node);
    expect(result.fillStyle).toBeUndefined();
    expect(result.fills).toEqual(["#ff0000"]);
  });

  it("skips fillStyle when fillStyleId is not a string", async () => {
    const node = {
      fills: [{ type: "SOLID", color: { r: 0, g: 0, b: 1 } }],
      fillStyleId: Symbol(),
    };
    const result = await serializeStyles(node);
    expect(result.fillStyle).toBeUndefined();
  });

  it("includes strokes and strokeStyle", async () => {
    mockGetStyleByIdAsync = async (id) => (id === "s-1" ? { name: "Border" } : null);
    const node = {
      strokes: [{ type: "SOLID", color: { r: 0, g: 0, b: 0 } }],
      strokeStyleId: "s-1",
    };
    const result = await serializeStyles(node);
    expect(result.strokeStyle).toBe("Border");
    expect(result.strokes).toEqual(["#000000"]);
  });

  it("omits cornerRadius when value is 0", async () => {
    const result = await serializeStyles({ cornerRadius: 0 });
    expect(result.cornerRadius).toBeUndefined();
  });

  it("includes cornerRadius when non-zero", async () => {
    const result = await serializeStyles({ cornerRadius: 8 });
    expect(result.cornerRadius).toBe(8);
  });

  it("sets cornerRadius to 'mixed' for symbol", async () => {
    const result = await serializeStyles({ cornerRadius: Symbol() });
    expect(result.cornerRadius).toBe("mixed");
  });

  it("includes padding when paddingLeft is present", async () => {
    const node = { paddingLeft: 10, paddingRight: 20, paddingTop: 5, paddingBottom: 15 };
    const result = await serializeStyles(node);
    expect(result.padding).toEqual({ top: 5, right: 20, bottom: 15, left: 10 });
  });

  it("includes effects when non-empty", async () => {
    const effects = [{ type: "DROP_SHADOW", radius: 4, color: { r: 0, g: 0, b: 0, a: 0.25 } }];
    const result = await serializeStyles({ effects });
    expect(result.effects).toEqual(effects);
  });

  it("omits effects when empty array — no noise", async () => {
    const result = await serializeStyles({ effects: [] });
    expect(result.effects).toBeUndefined();
  });

  it("omits effects when absent", async () => {
    const result = await serializeStyles({ id: "x" });
    expect(result.effects).toBeUndefined();
  });

  it("includes strokeWeight and strokeAlign when strokes are present", async () => {
    const node = {
      strokes: [{ type: "SOLID", color: { r: 0, g: 0, b: 0 } }],
      strokeWeight: 2,
      strokeAlign: "INSIDE",
    };
    const result = await serializeStyles(node);
    expect(result.strokeWeight).toBe(2);
    expect(result.strokeAlign).toBe("INSIDE");
  });

  it("omits strokeWeight/strokeAlign when strokes array is empty — no noise", async () => {
    const node = {
      strokes: [],
      strokeWeight: 1,
      strokeAlign: "CENTER",
    };
    const result = await serializeStyles(node);
    expect(result.strokeWeight).toBeUndefined();
    expect(result.strokeAlign).toBeUndefined();
  });

  it("emits strokeWeight:'mixed' when strokeWeight is a Figma mixed symbol", async () => {
    const node = {
      strokes: [{ type: "SOLID", color: { r: 1, g: 1, b: 1 } }],
      strokeWeight: Symbol(),
      strokeAlign: "OUTSIDE",
    };
    const result = await serializeStyles(node);
    expect(result.strokeWeight).toBe("mixed");
    expect(result.strokeAlign).toBe("OUTSIDE");
  });
});

// ── serializeText ─────────────────────────────────────────────────────────────

describe("serializeText", () => {
  const makeBase = () => ({ id: "t1", name: "Text", type: "TEXT", bounds: undefined, styles: {} });

  it("handles mixed font name", async () => {
    const node = {
      fontName: Symbol(),
      fontSize: 16,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      characters: "hello",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.fontFamily).toBe("mixed");
    expect(result.styles.fontStyle).toBe("mixed");
  });

  it("handles regular font name", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      characters: "hello",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.fontFamily).toBe("Inter");
    expect(result.styles.fontStyle).toBe("Regular");
    expect(result.characters).toBe("hello");
  });

  it("includes textStyle when textStyleId resolves", async () => {
    mockGetStyleByIdAsync = async (id) => (id === "ts-1" ? { name: "Heading 1" } : null);
    const node = {
      fontName: { family: "Inter", style: "Bold" },
      fontSize: 32,
      fontWeight: 700,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      textStyleId: "ts-1",
      characters: "Title",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.textStyle).toBe("Heading 1");
  });

  it("omits textStyle when textStyleId is not a string", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      textStyleId: Symbol(),
      characters: "hi",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.textStyle).toBeUndefined();
  });

  it("serializes mixed text properties", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: Symbol(),
      fontWeight: Symbol(),
      textDecoration: Symbol(),
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: Symbol(),
      characters: "mixed",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.fontSize).toBe("mixed");
    expect(result.styles.fontWeight).toBe("mixed");
    expect(result.styles.textDecoration).toBe("mixed");
    expect(result.styles.textAlignHorizontal).toBe("mixed");
  });

  it("omits textDecoration when value is NONE", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      characters: "plain",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.textDecoration).toBeUndefined();
  });

  it("includes textDecoration when not NONE", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "UNDERLINE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      characters: "underlined",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.textDecoration).toBe("UNDERLINE");
  });
});

// ── serializeNode ─────────────────────────────────────────────────────────────

describe("serializeNode", () => {
  it("serializes a plain node with bounds", async () => {
    const node = { id: "1:1", name: "Box", type: "RECTANGLE", x: 0, y: 0, width: 100, height: 50 };
    const result = await serializeNode(node);
    expect(result.id).toBe("1:1");
    expect(result.type).toBe("RECTANGLE");
    expect(result.bounds).toEqual({ x: 0, y: 0, width: 100, height: 50 });
  });

  it("serializes a TEXT node", async () => {
    const node = {
      id: "1:2",
      name: "Label",
      type: "TEXT",
      x: 0, y: 0, width: 50, height: 20,
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      characters: "Hello",
    };
    const result = await serializeNode(node);
    expect(result.type).toBe("TEXT");
    expect(result.characters).toBe("Hello");
  });

  it("recursively serializes children", async () => {
    const node = {
      id: "1:3",
      name: "Frame",
      type: "FRAME",
      x: 0, y: 0, width: 200, height: 200,
      children: [
        { id: "1:4", name: "Child", type: "RECTANGLE", x: 10, y: 10, width: 50, height: 50 },
      ],
    };
    const result = await serializeNode(node);
    expect(result.children).toHaveLength(1);
    expect(result.children[0].id).toBe("1:4");
  });

  it("resolves an INSTANCE's main component name + key + remote inline (5A, fix #2)", async () => {
    const node = {
      id: "1:5", name: "Submit", type: "INSTANCE", x: 0, y: 0, width: 80, height: 32,
      children: [],
      getMainComponentAsync: async () => ({ key: "abc123", name: "Button/Primary", remote: true }),
    };
    const result = await serializeNode(node);
    expect(result.mainComponent).toEqual({ key: "abc123", name: "Button/Primary", remote: true });
    // self-contained: no follow-up needed to learn which component this is
  });

  it("emits remote:false for a local component (non-library)", async () => {
    const node = {
      id: "1:5b", name: "Local", type: "INSTANCE", x: 0, y: 0, width: 80, height: 32,
      children: [],
      getMainComponentAsync: async () => ({ key: "local-key", name: "LocalBtn" }),
      // no remote property → defaults to false
    };
    const result = await serializeNode(node);
    expect(result.mainComponent.remote).toBe(false);
  });

  it("emits componentProperties on INSTANCE FLATTENED to bare values (fix — dedupeComponents consistency)", async () => {
    // The serializer now flattens {Type:{type:"VARIANT",value:"icon"}} → {Type:"icon"}
    // (bare value), matching the dedupeComponents path in read-document.ts so consumers
    // see ONE consistent shape regardless of which path produced the node.
    const node = {
      id: "1:7", name: "Icon", type: "INSTANCE", x: 0, y: 0, width: 24, height: 24,
      children: [],
      getMainComponentAsync: async () => ({ key: "icon-key", name: "Icon/Arrow", remote: false }),
      componentProperties: {
        "Type": { value: "chevron-right", type: "VARIANT" },
        "Size": { value: "24", type: "VARIANT" },
      },
    };
    const result = await serializeNode(node);
    expect(result.componentProperties).toBeDefined();
    // Bare values — NOT nested objects with a .value property
    expect(result.componentProperties["Type"]).toBe("chevron-right");
    expect(result.componentProperties["Size"]).toBe("24");
    // Confirm flattening: no .type sub-key present
    expect(typeof result.componentProperties["Type"]).toBe("string");
  });

  it("omits componentProperties when empty or absent — no noise", async () => {
    const node = {
      id: "1:8", name: "Basic", type: "INSTANCE", x: 0, y: 0, width: 80, height: 32,
      children: [],
      getMainComponentAsync: async () => ({ key: "k", name: "Btn", remote: false }),
      componentProperties: {},
    };
    const result = await serializeNode(node);
    expect(result.componentProperties).toBeUndefined();
  });

  it("emits opacity when != 1", async () => {
    const node = { id: "1:9", name: "Faded", type: "RECTANGLE", x: 0, y: 0, width: 100, height: 50, opacity: 0.5 };
    const result = await serializeNode(node);
    expect(result.opacity).toBe(0.5);
  });

  it("omits opacity when == 1 — no noise", async () => {
    const node = { id: "1:10", name: "Full", type: "RECTANGLE", x: 0, y: 0, width: 100, height: 50, opacity: 1 };
    const result = await serializeNode(node);
    expect(result.opacity).toBeUndefined();
  });

  it("emits visible:false when node is hidden", async () => {
    const node = { id: "1:11", name: "Hidden", type: "RECTANGLE", x: 0, y: 0, width: 100, height: 50, visible: false };
    const result = await serializeNode(node);
    expect(result.visible).toBe(false);
  });

  it("omits visible when true — no noise", async () => {
    const node = { id: "1:12", name: "Shown", type: "RECTANGLE", x: 0, y: 0, width: 100, height: 50, visible: true };
    const result = await serializeNode(node);
    expect(result.visible).toBeUndefined();
  });

  it("omits mainComponent when no main component resolves", async () => {
    const node = {
      id: "1:6", name: "Detached", type: "INSTANCE", x: 0, y: 0, width: 80, height: 32,
      children: [],
      getMainComponentAsync: async () => null,
    };
    const result = await serializeNode(node);
    expect(result.mainComponent).toBeUndefined();
  });

  // BYTE-IDENTITY SNAPSHOT: a plain FRAME node must serialize to EXACTLY the expected
  // object — no unexpected keys (opacity, visible, componentProperties, effects, mainComponent).
  // This locks the additive-only contract: any accidental always-emit of a conditional
  // field will be caught here.
  it("BYTE-IDENTITY: plain FRAME node has NO unexpected keys (additive-only contract)", async () => {
    const node = {
      id: "99:1",
      name: "Wrapper",
      type: "FRAME",
      x: 10,
      y: 20,
      width: 400,
      height: 300,
      // Deliberately omit: opacity, visible, effects, fills, children
    };
    const result = await serializeNode(node);
    expect(result).toEqual({
      id: "99:1",
      name: "Wrapper",
      type: "FRAME",
      bounds: { x: 10, y: 20, width: 400, height: 300 },
      styles: {},
      // The following keys must NOT appear:
      // opacity, visible, componentProperties, effects, mainComponent, children
    });
    expect(result.opacity).toBeUndefined();
    expect(result.visible).toBeUndefined();
    expect(result.componentProperties).toBeUndefined();
    expect(result.mainComponent).toBeUndefined();
    expect(result.children).toBeUndefined();
  });

  // BYTE-IDENTITY SNAPSHOT: an INSTANCE must include mainComponent:{key,name,remote} only.
  // Confirm shape is exactly {key, name, remote} — no extra fields leak from the
  // raw Figma component object.
  it("BYTE-IDENTITY: INSTANCE mainComponent shape is exactly {key, name, remote}", async () => {
    const node = {
      id: "99:2",
      name: "ButtonInstance",
      type: "INSTANCE",
      x: 0,
      y: 0,
      width: 120,
      height: 40,
      children: [],
      getMainComponentAsync: async () => ({
        key: "btn-key-abc",
        name: "Button/Primary",
        remote: true,
        // Extra fields on the raw Figma component object that must NOT leak:
        id: "comp:999",
        type: "COMPONENT",
        parent: {},
      }),
    };
    const result = await serializeNode(node);
    expect(result.mainComponent).toEqual({
      key: "btn-key-abc",
      name: "Button/Primary",
      remote: true,
    });
    // Confirm no extra fields leaked
    expect(Object.keys(result.mainComponent)).toEqual(["key", "name", "remote"]);
  });

  // Issue #29: an INSTANCE read must surface the resolved master node id as a
  // top-level `mainComponentId`, so a reader can reference/reuse a file-local
  // master without a follow-up round-trip. The id lives at top level — NOT inside
  // `mainComponent` — so the byte-identity guard above stays exactly {key,name,remote}.
  it("surfaces the master node id as top-level mainComponentId (issue #29)", async () => {
    const node = {
      id: "99:3",
      name: "ButtonInstance",
      type: "INSTANCE",
      x: 0,
      y: 0,
      width: 120,
      height: 40,
      children: [],
      getMainComponentAsync: async () => ({
        key: "btn-key-abc",
        name: "Button/Primary",
        remote: false,
        id: "comp:999",
      }),
    };
    const result = await serializeNode(node);
    expect(result.mainComponentId).toBe("comp:999");
    // mainComponent itself stays exactly {key,name,remote} — the id is top-level only.
    expect(Object.keys(result.mainComponent)).toEqual(["key", "name", "remote"]);
  });

  // A detached instance (no resolvable master) must omit mainComponentId entirely
  // rather than emit null/undefined — keeps payloads compact and diff-friendly.
  it("omits mainComponentId when no master resolves (issue #29)", async () => {
    const node = {
      id: "99:4",
      name: "Detached",
      type: "INSTANCE",
      x: 0,
      y: 0,
      width: 10,
      height: 10,
      children: [],
      getMainComponentAsync: async () => null,
    };
    const result = await serializeNode(node);
    expect(result.mainComponentId).toBeUndefined();
    expect(result.mainComponent).toBeUndefined();
  });
});
