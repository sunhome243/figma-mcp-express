/// <reference types="@figma/plugin-typings" />
//
// Compile-only assertion that every NEW Figma Plugin API property/method this PR
// uses actually exists on the real typed surface (@figma/plugin-typings), with the
// right value types. The runtime handlers type nodes as `any`, which silently opts
// OUT of this check — this file opts back IN. It has no runtime/tests; `tsc --noEmit`
// is the verification. If Figma renames/removes any of these, this fails to compile.

// ── Layout (Phase 2): GRID mode, grid gaps, min/max, extra auto-layout props ──
export function _assertLayout(f: FrameNode): void {
  f.layoutMode = "GRID";
  f.gridRowCount = 2;
  f.gridColumnCount = 3;
  f.gridRowGap = 8;
  f.gridColumnGap = 16;
  f.counterAxisAlignContent = "SPACE_BETWEEN";
  f.overflowDirection = "BOTH";
  f.strokesIncludedInLayout = true;
  f.itemReverseZIndex = true;
  f.minWidth = 100;
  f.maxWidth = 400;
  f.minHeight = 50;
  f.maxHeight = 800;
  f.minWidth = null;
  f.setBoundVariable("gridRowGap", null);
  f.setBoundVariable("counterAxisSpacing", null);
}

// ── Node creation (Phase 3) ──
export function _assertCreation(): void {
  const line: LineNode = figma.createLine();
  line.strokeCap = "ROUND";
  line.strokeWeight = 2;
  const poly: PolygonNode = figma.createPolygon();
  poly.pointCount = 6;
  const star: StarNode = figma.createStar();
  star.pointCount = 5;
  star.innerRadius = 0.4;
  const svg: FrameNode = figma.createNodeFromSvg("<svg></svg>");
  svg.name = "icon";
  const table: TableNode = figma.createTable(2, 3);
  const cell: TableCellNode = table.cellAt(0, 0);
  cell.text.characters = "A";
}

// ── ImagePaint (Phase 4): rotation/scalingFactor/imageTransform + filters ──
export function _assertImagePaint(): ImagePaint {
  const filters: ImageFilters = {
    exposure: 0.5, contrast: -0.2, saturation: 0.1,
    temperature: 0, tint: 0, highlights: 0.3, shadows: -0.1,
  };
  const transform: Transform = [[1, 0, 0], [0, 1, 0]];
  return {
    type: "IMAGE",
    imageHash: "abc",
    scaleMode: "TILE",
    rotation: 90,
    scalingFactor: 2,
    imageTransform: transform,
    filters,
  };
}

// ── Text (Phase 5): whole-node props + per-range setRange* methods ──
export function _assertText(t: TextNode): void {
  t.textTruncation = "ENDING";
  t.maxLines = 2;
  t.maxLines = null;
  t.paragraphIndent = 12;
  t.paragraphSpacing = 8;
  t.listSpacing = 4;
  t.leadingTrim = "CAP_HEIGHT";
  t.hangingPunctuation = true;
  t.hangingList = true;
  void t.setTextStyleIdAsync("S:1");

  const fonts: FontName[] = t.getRangeAllFontNames(0, 5);
  t.setRangeFontName(0, 5, fonts[0]);
  t.setRangeFontSize(0, 5, 18);
  t.setRangeFills(0, 5, []);
  t.setRangeTextCase(0, 5, "UPPER");
  t.setRangeTextDecoration(0, 5, "UNDERLINE");
  t.setRangeLetterSpacing(0, 5, { value: 2, unit: "PIXELS" });
  t.setRangeLineHeight(0, 5, { unit: "AUTO" });
  const urlLink: HyperlinkTarget = { type: "URL", value: "https://x.com" };
  t.setRangeHyperlink(0, 5, urlLink);
  t.setRangeHyperlink(0, 5, null);
  const list: TextListOptions = { type: "ORDERED" };
  t.setRangeListOptions(0, 5, list);
  t.setRangeIndentation(0, 5, 2);
}

// ── Variables (Phase 6): scopes, codeSyntax, hiddenFromPublishing, mode ops ──
export function _assertVariables(v: Variable, c: VariableCollection): void {
  v.name = "color/primary";
  const scopes: VariableScope[] = ["TEXT_FILL", "FRAME_FILL", "ALL_SCOPES"];
  v.scopes = scopes;
  v.hiddenFromPublishing = true;
  const platform: CodeSyntaxPlatform = "WEB";
  v.setVariableCodeSyntax(platform, "colorPrimary");
  v.setVariableCodeSyntax("iOS", "ColorPrimary");

  c.name = "Design Tokens";
  c.hiddenFromPublishing = true;
  c.renameMode("mode:1", "Light");
  c.removeMode("mode:1");
}

// ── Constraints (Phase 7): promoted set_constraints ──
export function _assertConstraints(n: SceneNode & ConstraintMixin): void {
  const cons: Constraints = { horizontal: "STRETCH", vertical: "CENTER" };
  n.constraints = cons;
}
