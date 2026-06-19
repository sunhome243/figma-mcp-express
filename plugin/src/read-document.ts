import {
  serializeNode,
  getBounds,
  serializeStyles,
  isMixed,
  deduplicateStyles,
  serializeAutoLayout,
  serializeCodegenTokens,
  serializeComponentRef,
  prewarmReadCaches,
  type SerializeCaches,
} from "./serializers";
import { makeProgress } from "./progress";

// One per-read cache set, threaded into every serializeNode + serializeComponentRef
// call in a single handler so a style/component shared by many nodes resolves once
// (7R-2). Created fresh per request — never module-global — so a rename can't serve
// a stale name on a later read.
const makeReadCaches = (): SerializeCaches => ({
  styles: new Map(),
  components: new Map(),
});

// Default depth cap for get_node when no `depth` param is given. Generous enough
// that any real component/screen subtree serializes in full, but finite so an
// accidental call on a giant root doesn't walk the entire page. Overridable per
// call (pass a larger depth or Infinity for an unbounded read).
const DEFAULT_GET_NODE_DEPTH = 50;

const bytesToBase64 = (bytes: Uint8Array): string => {
  if (typeof figma.base64Encode === "function") return figma.base64Encode(bytes);
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
};

export const handleReadDocumentRequest = async (request: any) => {
  switch (request.type) {
    case "get_document": {
      // serializeNode walks the entire page in one recursive call; thread a tick
      // through onVisit so a large page keeps the Go-bridge inactivity timer alive.
      const tick = makeProgress(request.requestId, "get_document");
      const raw = await serializeNode(figma.currentPage, makeReadCaches(), tick);
      const { tree, globalVars } = deduplicateStyles(raw);
      return {
        type: request.type,
        requestId: request.requestId,
        data: globalVars ? { ...tree, globalVars } : tree,
      };
    }

    case "get_selection": {
      const caches = makeReadCaches();
      return {
        type: request.type,
        requestId: request.requestId,
        data: await Promise.all(
          figma.currentPage.selection.map((node) => serializeNode(node, caches)),
        ),
      };
    }

    case "get_image_by_hash": {
      const hash = request.params && request.params.hash;
      if (!hash) throw new Error("hash is required");
      const image = figma.getImageByHash(String(hash));
      if (!image) {
        return { type: request.type, requestId: request.requestId, data: { hash, image: null } };
      }
      const [size, bytes] = await Promise.all([image.getSizeAsync(), image.getBytesAsync()]);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { hash: image.hash, width: size.width, height: size.height, bytesBase64: bytesToBase64(bytes) },
      };
    }

    case "get_file_thumbnail": {
      if (typeof figma.getFileThumbnailNodeAsync !== "function") {
        throw new Error("getFileThumbnailNodeAsync is unavailable in this Figma host");
      }
      const node = await figma.getFileThumbnailNodeAsync();
      return {
        type: request.type,
        requestId: request.requestId,
        data: node ? { nodeId: node.id, name: node.name, type: node.type } : { nodeId: null },
      };
    }

    case "get_dev_resources": {
      const nodeId = request.params && request.params.nodeId;
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(String(nodeId)) as any;
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (typeof node.getDevResourcesAsync !== "function") {
        throw new Error(`Node ${nodeId} does not support dev resources`);
      }
      const resources = await node.getDevResourcesAsync({ includeChildren: !!request.params?.includeChildren });
      return { type: request.type, requestId: request.requestId, data: { nodeId, resources } };
    }

    case "resolve_variable_for_consumer": {
      const p = request.params || {};
      if (!p.variableId) throw new Error("variableId is required");
      if (!p.nodeId) throw new Error("nodeId is required");
      const variable = await figma.variables.getVariableByIdAsync(p.variableId);
      if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
      const node = await figma.getNodeByIdAsync(String(p.nodeId)) as SceneNode | null;
      if (!node) throw new Error(`Node not found: ${p.nodeId}`);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { variableId: variable.id, nodeId: node.id, resolved: variable.resolveForConsumer(node) },
      };
    }

    case "get_node": {
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeIds is required for get_node");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node || node.type === "DOCUMENT")
        throw new Error(`Node not found: ${nodeId}`);
      // Optional depth limit: when provided, mirrors the depth-limiting logic of
      // get_design_context's serializeWithDepth (same mechanism, reused exactly).
      // When omitted, a generous default cap (DEFAULT_GET_NODE_DEPTH) guards
      // against an accidental whole-page Infinity walk while staying deep enough
      // that real component/screen trees serialize in full. The param stays
      // overridable (pass a larger depth, or Infinity, for an unbounded read).
      const depth =
        request.params && request.params.depth != null
          ? request.params.depth
          : DEFAULT_GET_NODE_DEPTH;
      const tickNode = makeProgress(request.requestId, "get_node");
      const caches = makeReadCaches();

      // Prefetch the unique style + main-component lookups in parallel so the walk
      // below hits the cache instead of awaiting each distinct id serially.
      await prewarmReadCaches([node], caches, { maxDepth: depth });

      // Single depth-bounded walk. serializeNode applies the depth cap + the
      // { childCount } truncation itself, so there is no second re-walk: the old
      // serializeNodeWithDepth called serializeNode(n) — which already recursed the
      // WHOLE subtree — once per level (O(N·D)) and re-fetched every child by id.
      // That double-serialization is why get_node(depth:1) on a giant node still
      // timed out (it fully serialized the subtree before truncating).
      return {
        type: request.type,
        requestId: request.requestId,
        data: await serializeNode(node, caches, tickNode, { maxDepth: depth }),
      };
    }

    case "get_nodes_info": {
      if (!request.nodeIds || request.nodeIds.length === 0)
        throw new Error("nodeIds is required for get_nodes_info");
      const nodes = await Promise.all(
        request.nodeIds.map((id: string) => figma.getNodeByIdAsync(id)),
      );
      // Depth cap: get_nodes_info previously did an UNBOUNDED full recursion — the
      // same latent timeout get_node had on a giant node. Default to the same
      // generous cap as get_node (callers may override via params.depth). Normal
      // trees (well under the cap) are byte-identical to before.
      const depth =
        request.params && request.params.depth != null
          ? request.params.depth
          : DEFAULT_GET_NODE_DEPTH;
      // Shared tick counter across all node serializations so the 800-node
      // threshold is accumulated globally — mirrors get_design_context's shared
      // tickContext across selection nodes.
      const tickInfo = makeProgress(request.requestId, "get_nodes_info");
      const caches = makeReadCaches();

      // Prefetch unique style + main-component lookups across all requested nodes
      // in parallel before the serial-per-node walk.
      await prewarmReadCaches(
        nodes.filter((n) => n !== null && n.type !== "DOCUMENT"),
        caches,
        { maxDepth: depth },
      );

      // The old serializeInfoNode re-walked the tree calling serializeNode per
      // level — 100% redundant, since serializeNode already recurses the whole
      // subtree. One serializeNode call per requested node yields byte-identical
      // output (now depth-bounded) without the per-level re-serialize + re-fetch.
      return {
        type: request.type,
        requestId: request.requestId,
        data: await Promise.all(
          nodes
            .filter((n) => n !== null && n.type !== "DOCUMENT")
            .map((n) => serializeNode(n, caches, tickInfo, { maxDepth: depth })),
        ),
      };
    }

    case "get_design_context": {
      const depth =
        request.params && request.params.depth != null
          ? request.params.depth
          : 2;
      const detail = (request.params && request.params.detail) || "full";
      const dedupeComponents = !!(request.params && request.params.dedupeComponents);
      const codeConnectMap =
        (request.params && request.params.codeConnectMap) || {};
      const componentDefs = new Map<string, any>();

      // enrichForCodegen augments an already-fully-serialized node with the
      // codegen-only signals an LLM needs: auto-layout config, bound design
      // tokens, the main-component key/name, and any matching Code-Connect entry.
      // It mutates nothing on the input — returns a new object.
      // One token-resolution cache per get_design_context call — distinct nodes
      // binding the same token resolve it once, not once per node.
      const tokenCache = new Map<string, string | undefined>();
      // Per-read style/component caches threaded through every serializeNode +
      // serializeComponentRef call below (7R-2), so a style or main component
      // shared by many nodes resolves once for the whole context read.
      const caches = makeReadCaches();
      // Cooperative yield counter for the deep serialization walk.
      // Uses makeProgress so the counter is shared across recursive calls and
      // progress_update messages reset the Go-bridge timeout every 800 nodes.
      // Yields every 800 nodes; avoids jams on large trees.
      const tickContext = makeProgress(request.requestId, "get_design_context");

      const enrichForCodegen = async (node: any, serialized: any): Promise<any> => {
        const result = Object.assign({}, serialized);
        const autoLayout = serializeAutoLayout(node);
        if (autoLayout) result.autoLayout = autoLayout;
        const tokens = await serializeCodegenTokens(node, tokenCache);
        if (tokens) result.tokens = tokens;
        const componentRef = await serializeComponentRef(node, caches.components);
        if (componentRef) {
          // componentRef is Code-Connect-keyed ({key,name,remote}); the master node
          // id rides on serializeNode's top-level mainComponentId (issue #29), not here.
          const { id: _masterId, ...refForCodegen } = componentRef;
          result.componentRef = refForCodegen;
          const cc = codeConnectMap[componentRef.key];
          if (cc !== undefined) result.codeConnect = cc;
        }
        return result;
      };

      const serializeForDetail = async (n: any) => {
        const base = { id: n.id, name: n.name, type: n.type, bounds: getBounds(n) };
        if (detail === "minimal") return base;
        const styles = await serializeStyles(n, caches.styles);
        const result: any = Object.assign({}, base);
        if (Object.keys(styles).length > 0) result.styles = styles;
        if ("opacity" in n && n.opacity !== 1) result.opacity = n.opacity;
        if ("visible" in n && !n.visible) result.visible = false;
        if (detail === "compact") return result;
        return await serializeNode(n, caches);
      };

      const extractInstanceOverrides = async (
        instanceNode: any,
        componentNode: any,
      ): Promise<{ id: string; name: string; type: string; characters?: string; mainComponentId?: string | null; visible?: boolean; opacity?: number; fills?: any }[]> => {
        const overrides: any[] = [];
        if (!instanceNode?.children || !componentNode?.children) return overrides;
        for (let i = 0; i < instanceNode.children.length; i++) {
          const instChild = instanceNode.children[i];
          if (!instChild) continue;
          // Match instance children to component children by the last `;`-segment of the
          // instance-descendant id. Figma encodes instance-descendant ids as
          // `I<instanceId>;<componentChildId>` (e.g. `I216:103729;13:126572`), with nested
          // instances chaining further segments. The last segment is the component child's
          // own full `A:B` id — collision-free across page-prefixes. Splitting on `:` instead
          // would yield only the trailing counter (`126572`), which collides across pages and
          // silently mis-pairs siblings. Fall back to positional index when id lookup fails.
          const instSuffix = instChild.id.split(";").pop();
          const compChild =
            componentNode.children.find((c: any) => c.id === instSuffix) ??
            componentNode.children[i];
          if (!compChild) continue;

          // Detect property overrides (visible, opacity, fills) for all node types
          const propChanges: any = {};
          if ("visible" in instChild && "visible" in compChild && instChild.visible !== compChild.visible) {
            propChanges.visible = instChild.visible;
          }
          if ("opacity" in instChild && "opacity" in compChild && instChild.opacity !== compChild.opacity) {
            propChanges.opacity = instChild.opacity;
          }
          if ("fills" in instChild && "fills" in compChild && !isMixed(instChild.fills) && !isMixed(compChild.fills)) {
            if (JSON.stringify(instChild.fills) !== JSON.stringify(compChild.fills)) {
              propChanges.fills = instChild.fills;
            }
          }

          if (instChild.type === "TEXT") {
            const override: any = { id: instChild.id, name: instChild.name, type: "TEXT" };
            let hasChange = false;
            if (instChild.characters !== compChild.characters) {
              override.characters = instChild.characters;
              hasChange = true;
            }
            if (Object.keys(propChanges).length > 0) {
              Object.assign(override, propChanges);
              hasChange = true;
            }
            if (hasChange) overrides.push(override);
            continue;
          }

          if (instChild.type === "INSTANCE") {
            const [nestedMc, compMc] = await Promise.all([
              instChild.getMainComponentAsync(),
              compChild.type === "INSTANCE" ? compChild.getMainComponentAsync() : Promise.resolve(null),
            ]);
            if (nestedMc?.id !== compMc?.id) {
              const override: any = { id: instChild.id, name: instChild.name, type: "INSTANCE", mainComponentId: nestedMc?.id ?? null };
              if (Object.keys(propChanges).length > 0) Object.assign(override, propChanges);
              overrides.push(override);
              continue;
            }
            if (Object.keys(propChanges).length > 0) {
              overrides.push({ id: instChild.id, name: instChild.name, type: "INSTANCE", mainComponentId: nestedMc?.id ?? null, ...propChanges });
            }
            if (nestedMc) overrides.push(...await extractInstanceOverrides(instChild, nestedMc));
            continue;
          }

          if (Object.keys(propChanges).length > 0) {
            overrides.push({ id: instChild.id, name: instChild.name, type: instChild.type, ...propChanges });
          }
          if ("children" in instChild) {
            overrides.push(...await extractInstanceOverrides(instChild, compChild));
          }
        }
        return overrides;
      };

      const serializeWithDepth = async (node: any, currentDepth: number): Promise<any> => {
        await tickContext();
        if (dedupeComponents && node.type === "INSTANCE") {
          const mc = await node.getMainComponentAsync();
          if (mc && !componentDefs.has(mc.id)) {
            componentDefs.set(mc.id, await serializeNode(mc, caches));
          }
          const props: Record<string, any> = {};
          if (node.componentProperties) {
            for (const [key, prop] of Object.entries(node.componentProperties)) {
              props[key] = (prop as any).value;
            }
          }
          const result: any = {
            id: node.id,
            name: node.name,
            type: node.type,
            bounds: getBounds(node),
            mainComponentId: mc?.id ?? null,
          };
          if (Object.keys(props).length > 0) result.componentProperties = props;
          const overrides = await extractInstanceOverrides(node, mc);
          if (overrides.length > 0) result.overrides = overrides;
          return result;
        }
        if (detail === "full" || detail === "codegen") {
          // Fast single-walk path (the common case): serializeNode applies the
          // depth cap + { childCount } truncation AND — for codegen — the per-node
          // enrichment, appended AFTER children so key order is preserved, all in
          // ONE pass. Replaces the old serializeNode(full-subtree)-then-re-walk
          // that re-serialized every subtree once per level (O(N·D)) and re-fetched
          // each child by id.
          if (!dedupeComponents) {
            return await serializeNode(node, caches, tickContext, {
              maxDepth: depth,
              currentDepth,
              enrich: detail === "codegen" ? enrichForCodegen : undefined,
            });
          }
          // dedupeComponents=true: keep the per-level walk so a nested INSTANCE
          // still hits the dedupe branch above (a single serializeNode walk would
          // serialize it in full instead of compacting it). Bound this call to
          // maxDepth:1 — we use `serialized` ONLY for the node's own fields and the
          // direct-child id list (the re-walk below replaces children), so a deeper
          // serialization is pure waste. Without the cap this call serialized the
          // ENTIRE subtree (the same unbounded-then-truncate cost the get_node fix
          // removed); the cap makes it byte-identical at ~2N total work.
          const serialized = await serializeNode(node, caches, undefined, { maxDepth: 1 });
          let result: any;
          if (currentDepth >= depth && serialized.children) {
            result = Object.assign({}, serialized, {
              children: undefined,
              childCount: node.children ? node.children.length : 0,
            });
          } else if (serialized.children) {
            const childNodes = await Promise.all(
              serialized.children.map((child: any) =>
                figma.getNodeByIdAsync(child.id),
              ),
            );
            const serializedChildren = await Promise.all(
              childNodes
                .filter((n) => n !== null && n.type !== "DOCUMENT")
                .map((n) => serializeWithDepth(n, currentDepth + 1)),
            );
            result = Object.assign({}, serialized, { children: serializedChildren });
          } else {
            result = serialized;
          }
          if (detail === "codegen") result = await enrichForCodegen(node, result);
          return result;
        }

        const serialized = await serializeForDetail(node);
        const hasChildren = "children" in node && node.children.length > 0;
        if (!hasChildren) return serialized;
        if (currentDepth >= depth) {
          return Object.assign({}, serialized, { childCount: node.children.length });
        }
        const serializedChildren = await Promise.all(
          node.children
            .filter((n: any) => n.type !== "DOCUMENT")
            .map((n: any) => serializeWithDepth(n, currentDepth + 1)),
        );
        return Object.assign({}, serialized, { children: serializedChildren });
      };

      // When a nodeId is supplied, scope to that subtree regardless of the current
      // selection (issue #34). Otherwise fall back to the selection, or the whole
      // page when nothing is selected.
      const scopeNodeId = request.params && request.params.nodeId;
      const selection = figma.currentPage.selection;
      let roots: any[];
      if (scopeNodeId) {
        const scoped = await figma.getNodeByIdAsync(scopeNodeId);
        if (!scoped) throw new Error(`Node not found: ${scopeNodeId}`);
        roots = [scoped];
      } else {
        roots =
          selection.length > 0 ? Array.from(selection) : [figma.currentPage];
      }
      // Prefetch the unique style / main-component / (codegen) token lookups across
      // all roots in parallel for the single-walk fast path (the dedupeComponents
      // and compact/minimal paths still resolve correctly inline — prewarm only
      // ever populates caches, never changes output).
      if (!dedupeComponents && (detail === "full" || detail === "codegen")) {
        await prewarmReadCaches(roots, caches, {
          maxDepth: depth,
          tokenCache: detail === "codegen" ? tokenCache : undefined,
        });
      }
      const rawContextNodes = await Promise.all(
        roots.map((node) => serializeWithDepth(node, 0)),
      );
      const { tree: dedupedNodes, globalVars } = deduplicateStyles({ children: rawContextNodes });
      const contextNodes = (dedupedNodes as any).children;
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          fileName: figma.root.name,
          currentPage: {
            id: figma.currentPage.id,
            name: figma.currentPage.name,
          },
          selectionCount: selection.length,
          context: contextNodes,
          ...(componentDefs.size > 0 ? { componentDefs: Object.fromEntries(componentDefs) } : {}),
          ...(globalVars ? { globalVars } : {}),
        },
      };
    }

    case "get_metadata":
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          fileName: figma.root.name,
          currentPageId: figma.currentPage.id,
          currentPageName: figma.currentPage.name,
          pageCount: figma.root.children.length,
          pages: figma.root.children.map((page) => ({
            id: page.id,
            name: page.name,
          })),
        },
      };

    case "get_pages":
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          currentPageId: figma.currentPage.id,
          pages: figma.root.children.map((page) => ({
            id: page.id,
            name: page.name,
          })),
        },
      };

    case "get_viewport":
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          center: { x: figma.viewport.center.x, y: figma.viewport.center.y },
          zoom: figma.viewport.zoom,
          bounds: {
            x: figma.viewport.bounds.x,
            y: figma.viewport.bounds.y,
            width: figma.viewport.bounds.width,
            height: figma.viewport.bounds.height,
          },
        },
      };

    case "get_fonts": {
      // Async + per-node tick: the walk covers the whole page, so yield + emit a
      // progress_update periodically to keep the Go-bridge inactivity timer alive.
      const tick = makeProgress(request.requestId, "get_fonts");
      const fontMap = new Map<string, any>();
      const collectFonts = async (n: any) => {
        await tick();
        if (n.type === "TEXT") {
          const fontName = n.fontName;
          if (typeof fontName !== "symbol" && fontName) {
            const key = `${fontName.family}::${fontName.style}`;
            if (!fontMap.has(key)) {
              fontMap.set(key, { family: fontName.family, style: fontName.style, nodeCount: 0 });
            }
            fontMap.get(key).nodeCount++;
          }
        }
        if ("children" in n) {
          for (const child of n.children) await collectFonts(child);
        }
      };
      await collectFonts(figma.currentPage);
      const fonts = Array.from(fontMap.values()).sort((a, b) => b.nodeCount - a.nodeCount);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { count: fonts.length, fonts },
      };
    }

    case "search_nodes": {
      const query = request.params && request.params.query
        ? request.params.query.toLowerCase()
        : "";
      const scopeNodeId = request.params && request.params.nodeId;
      const types = request.params && request.params.types ? request.params.types : [];
      const limit = request.params && request.params.limit ? request.params.limit : 50;
      const root = scopeNodeId
        ? await figma.getNodeByIdAsync(scopeNodeId)
        : figma.currentPage;
      if (!root) throw new Error(`Node not found: ${scopeNodeId}`);
      const results: any[] = [];
      const tick = makeProgress(request.requestId, "search_nodes");
      const search = async (n: any) => {
        if (results.length >= limit) return;
        await tick();
        if (n !== root) {
          const nameMatch = !query || n.name.toLowerCase().includes(query);
          const typeMatch = types.length === 0 || types.includes(n.type);
          if (nameMatch && typeMatch) {
            results.push({
              id: n.id,
              name: n.name,
              type: n.type,
              bounds: getBounds(n),
            });
          }
        }
        if (results.length < limit && "children" in n) {
          for (const child of n.children) await search(child);
        }
      };
      await search(root);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { count: results.length, nodes: results },
      };
    }

    case "get_reactions": {
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required for get_reactions");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node || node.type === "DOCUMENT") throw new Error(`Node not found: ${nodeId}`);
      const reactions = "reactions" in node ? node.reactions : [];
      return {
        type: request.type,
        requestId: request.requestId,
        data: { nodeId: node.id, name: node.name, reactions },
      };
    }

    case "scan_text_nodes": {
      const nodeId = request.params && request.params.nodeId;
      if (!nodeId) throw new Error("nodeId is required for scan_text_nodes");
      const root: any = await figma.getNodeByIdAsync(nodeId);
      if (!root) throw new Error(`Node not found: ${nodeId}`);
      // Native findAllWithCriteria runs the type-only traversal in C++ (~10-100×
      // a JS walk crossing the boundary per node) AND blocks the single thread
      // for a shorter total time, so Figma stutters less even without mid-scan
      // yields (7R-1). It returns DESCENDANTS in DFS pre-order, EXCLUDING root,
      // and (like the old walk) does NOT prune by visibility — so prepend root
      // when root itself is TEXT to keep results byte-identical to the manual walk.
      const toTextNode = (n: any) => ({
        id: n.id,
        name: n.name,
        characters: n.characters,
        fontSize: isMixed(n.fontSize) ? "mixed" : n.fontSize,
        fontName: isMixed(n.fontName) ? "mixed" : n.fontName,
      });
      // Heartbeat BEFORE the (native, fast) scan — resets the bridge inactivity
      // timer ahead of the work, preserving the old code's intent.
      figma.ui.postMessage({
        type: "progress_update",
        requestId: request.requestId,
        progress: 10,
        message: "Scanning text nodes...",
      });
      await new Promise((r) => setTimeout(r, 0));
      // NOTE: we deliberately do NOT set skipInvisibleInstanceChildren=true here.
      // The old manual walk had NO visibility pruning — it pushed every TEXT node,
      // including text inside invisible instances. Turning the flag on would make
      // native findAllWithCriteria skip invisible-instance children and drop that
      // text, diverging from the byte-identical contract. Leave it off for scans.
      // findAllWithCriteria exists only on container nodes — a leaf root (e.g. a
      // TEXT node passed as nodeId) lacks it, so fall back to root-only; the old
      // manual walk returned [root] for a TEXT root and [] otherwise.
      const found =
        typeof root.findAllWithCriteria === "function"
          ? root.findAllWithCriteria({ types: ["TEXT"] })
          : [];
      const ordered = root.type === "TEXT" ? [root, ...found] : found;
      const textNodes = ordered.map(toTextNode);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { count: textNodes.length, textNodes },
      };
    }

    case "scan_nodes_by_types": {
      const nodeId = request.params && request.params.nodeId;
      const types =
        request.params && request.params.types ? request.params.types : [];
      if (!nodeId)
        throw new Error("nodeId is required for scan_nodes_by_types");
      if (types.length === 0)
        throw new Error("types must be a non-empty array");
      const root: any = await figma.getNodeByIdAsync(nodeId);
      if (!root) throw new Error(`Node not found: ${nodeId}`);
      // Native traversal (7R-1) — see scan_text_nodes. The manual walk it
      // replaces pruned invisible subtrees (`if (!visible) return` skips the node
      // AND its descendants) and included root when root's type matched. Native
      // does neither, so to stay byte-identical we (a) prepend root when it
      // matches and is visible, and (b) drop any match that is itself hidden or
      // has a hidden ancestor up to root (the ancestor walk runs only on matches,
      // so most of the native speedup survives).
      const visibleThroughAncestors = (n: any) => {
        let cur: any = n;
        while (cur && cur !== root.parent) {
          if ("visible" in cur && !cur.visible) return false;
          if (cur === root) break;
          cur = cur.parent;
        }
        return true;
      };
      const toMatch = (n: any) => {
        // Emit both bbox (kept for back-compat) and bounds (same shape) so that consumers
        // of get_node / get_nodes_info can use either field without conditional checks.
        const bbox = {
          x: "x" in n ? n.x : 0,
          y: "y" in n ? n.y : 0,
          width: "width" in n ? n.width : 0,
          height: "height" in n ? n.height : 0,
        };
        return {
          id: n.id,
          name: n.name,
          type: n.type,
          bbox,
          bounds: bbox,
        };
      };
      // Heartbeat BEFORE the (native, fast) scan — resets the bridge inactivity
      // timer ahead of the work, preserving the old code's intent.
      figma.ui.postMessage({
        type: "progress_update",
        requestId: request.requestId,
        progress: 10,
        message: `Scanning for types: ${types.join(", ")}...`,
      });
      await new Promise((r) => setTimeout(r, 0));
      // findAllWithCriteria exists only on container nodes — a leaf root lacks it,
      // so fall back to root-only (the old manual walk returned [root] when the
      // leaf root's own type matched, [] otherwise — handled by rootMatches below).
      // Byte-identical-safe perf hint: this scan already drops invisible subtrees
      // (visibleThroughAncestors below), so skipping invisible instance children
      // during the native traversal cannot change the result set — it only avoids
      // descending trees we'd discard anyway. (scan_text_nodes deliberately omits
      // this; see its note.) Restored after the scan so it can't leak to later ops.
      let found: any[] = [];
      if (typeof root.findAllWithCriteria === "function") {
        const prevSkip = figma.skipInvisibleInstanceChildren;
        figma.skipInvisibleInstanceChildren = true;
        try {
          found = root.findAllWithCriteria({ types });
        } finally {
          figma.skipInvisibleInstanceChildren = prevSkip;
        }
      }
      const rootMatches =
        types.includes(root.type) && !("visible" in root && !root.visible);
      const matchingNodes = [
        ...(rootMatches ? [toMatch(root)] : []),
        ...found.filter(visibleThroughAncestors).map(toMatch),
      ];
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          count: matchingNodes.length,
          matchingNodes,
          searchedTypes: types,
        },
      };
    }

    default:
      return null;
  }
};
