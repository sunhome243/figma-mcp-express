import { owningPage } from "./node-utils";
import { makeSolidPaint } from "./write-helpers";

const OVERFLOW_DIRECTIONS = ["NONE", "HORIZONTAL", "VERTICAL", "BOTH"];

// Validate and normalize one action by its well-known type. A NODE (navigation) action's
// `navigation` and `transition` are non-optional in the Plugin API, so a minimal
// {type:"NODE", destinationId} would throw at setReactionsAsync — we require the destination
// and default the other two (NAVIGATE / no transition) so the common case just works.
//
// For NODE we spread the rest of the action through untouched, so OVERLAY-only fields
// (`overlayRelativePosition`), scroll/video/component reset flags (`resetScrollPosition`,
// `resetVideoPosition`, `resetInteractiveComponents`), and any `transition` (including
// directional transitions and advanced easings like GENTLE/BOUNCY/CUSTOM_SPRING) are
// preserved. URL likewise preserves `openInNewTab`.
//
// We additionally guard the required field of each well-known action type so the caller
// gets a clear error here instead of an opaque setReactionsAsync rejection. Unknown action
// types pass through unchanged (forward-compatible with future Plugin API additions).
function normalizeAction(action: any): Action {
  if (!action || typeof action !== "object") return action;
  const type: string = action.type ?? "";
  switch (type) {
    case "NODE":
      if (action.destinationId == null) {
        throw new Error(`Action of type "NODE" requires "destinationId"`);
      }
      return {
        ...action,
        navigation: action.navigation ?? "NAVIGATE",
        transition: action.transition ?? null,
      } as Action;
    case "URL":
      if (!action.url) throw new Error(`Action of type "URL" requires "url"`);
      return action as Action;
    case "SET_VARIABLE":
      if (action.variableId == null) {
        throw new Error(`Action of type "SET_VARIABLE" requires "variableId"`);
      }
      return action as Action;
    case "SET_VARIABLE_MODE":
      if (action.variableCollectionId == null || action.variableModeId == null) {
        throw new Error(
          `Action of type "SET_VARIABLE_MODE" requires "variableCollectionId" and "variableModeId"`
        );
      }
      return action as Action;
    case "CONDITIONAL":
      if (!Array.isArray(action.conditionalBlocks)) {
        throw new Error(`Action of type "CONDITIONAL" requires a "conditionalBlocks" array`);
      }
      return action as Action;
    case "UPDATE_MEDIA_RUNTIME":
      if (action.mediaAction == null) {
        throw new Error(`Action of type "UPDATE_MEDIA_RUNTIME" requires "mediaAction"`);
      }
      return action as Action;
    default:
      // BACK, CLOSE, and any unknown/future action types pass through unchanged.
      return action as Action;
  }
}

function buildReaction(r: any): Reaction {
  // A reaction with no trigger is silently dropped by Figma — reject it so the caller knows.
  if (r.trigger == null) {
    throw new Error(`Each reaction must have a "trigger" (got ${JSON.stringify(r.trigger)})`);
  }
  // `actions` (plural array) is the current API; `action` (singular) is deprecated — accept
  // either, and normalize each so required per-type fields are present before we forward.
  const rawActions: any[] = r.actions ?? (r.action != null ? [r.action] : []);
  const actions: Action[] = rawActions.map(normalizeAction);
  return { trigger: r.trigger, actions } as Reaction;
}

// The MCP framework may pass array params as a JSON string. Parse defensively.
function parseArray(v: any): any[] {
  if (Array.isArray(v)) return v;
  if (typeof v === "string") {
    try { return JSON.parse(v); } catch { return []; }
  }
  return [];
}

// setReactionsAsync is required when documentAccess is "dynamic-page".
// Fall back to direct assignment only when setReactionsAsync is unavailable (older Figma).
async function setReactions(node: any, reactions: Reaction[]): Promise<void> {
  if (typeof node.setReactionsAsync === "function") {
    await node.setReactionsAsync(reactions);
    return;
  }
  try {
    node.reactions = reactions;
  } catch (e) {
    throw new Error(`Failed to set reactions: ${e instanceof Error ? e.message : String(e)}`);
  }
}

export const handleWritePrototypeRequest = async (request: any) => {
  switch (request.type) {
    case "set_reactions": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (!("reactions" in node)) throw new Error(`Node ${nodeId} does not support reactions`);

      const incoming: Reaction[] = parseArray(p.reactions).map(buildReaction);
      const current: Reaction[] = (node as any).reactions;
      const final = p.mode === "append" ? [...current, ...incoming] : incoming;

      await setReactions(node, final);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: (node as any).name, reactionCount: final.length },
      };
    }

    case "remove_reactions": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (!("reactions" in node)) throw new Error(`Node ${nodeId} does not support reactions`);

      const current: Reaction[] = (node as any).reactions;
      let updated: Reaction[];
      if (p.indices == null) {
        // indices not provided → remove all
        updated = [];
      } else {
        const indices = parseArray(p.indices);
        if (indices.length === 0) {
          // indices provided but empty → remove all
          updated = [];
        } else {
          const toRemove = new Set<number>(indices);
          updated = current.filter((_: any, i: number) => !toRemove.has(i));
        }
      }

      await setReactions(node, updated);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: node.id,
          name: (node as any).name,
          removed: current.length - updated.length,
          reactionCount: updated.length,
        },
      };
    }

    case "set_prototype_start": {
      // Set the page's flow starting points. `prototypeStartNode` is read-only in the
      // Plugin API; the start of a prototype is controlled through page.flowStartingPoints.
      // There is no async setter — direct assignment of a new array is the documented path
      // (works under documentAccess:"dynamic-page").
      const p = request.params || {};
      const nodeIds: string[] = request.nodeIds ?? [];
      const mode = p.mode || "replace";

      // Clear mode: empty the page's flow starting points. This is the only path
      // to remove ALL start points — replace/append both require >=1 nodeId, so
      // without it a prototype can never be returned to "no defined entry". Targets
      // the page of the given nodeId(s) when provided, else the current page. When
      // several nodeIds are given they must share a page (matching the non-clear
      // path) — otherwise it's ambiguous which page to clear, so we error rather
      // than silently clear only the first node's page.
      if (mode === "clear") {
        let page: any = figma.currentPage;
        for (const id of nodeIds) {
          const node = await figma.getNodeByIdAsync(id);
          if (!node) throw new Error(`Node not found: ${id}`);
          const ancestor = owningPage(node);
          if (!ancestor) throw new Error(`Node ${id} is not on a page`);
          if (nodeIds[0] === id) page = ancestor;
          else if (page.id !== ancestor.id) {
            throw new Error("All clear-mode nodes must be on the same page");
          }
        }
        page.flowStartingPoints = [];
        figma.commitUndo();
        return {
          type: request.type,
          requestId: request.requestId,
          data: { pageId: page.id, pageName: page.name, flowStartingPoints: [] },
        };
      }

      // Remove mode: drop the given frames from the page's flow starting points and
      // keep the rest — the targeted inverse of append, for removing one start point
      // without re-listing the others. It's a cleanup op, so unlike replace/append it
      // TOLERATES an already-deleted frame: the page is resolved from the first nodeId
      // that still resolves (else the current page), then entries are filtered by id —
      // so a dangling start point pointing at a deleted frame is removable here, which
      // the strict resolve-every-node path below could not. Filtering by id also makes
      // remove incapable of damaging the wrong page: ids that aren't start points on the
      // resolved page simply don't match (a no-op), never a destructive change.
      if (mode === "remove") {
        if (nodeIds.length === 0) throw new Error("At least one nodeId is required");
        let page: any = figma.currentPage;
        for (const id of nodeIds) {
          const node = await figma.getNodeByIdAsync(id);
          const ancestor = node ? owningPage(node) : null;
          if (ancestor) {
            page = ancestor;
            break;
          }
        }
        const toRemove = new Set(nodeIds);
        const current: Array<{ nodeId: string; name: string }> = page.flowStartingPoints
          ? [...page.flowStartingPoints]
          : [];
        const final = current.filter((c) => !toRemove.has(c.nodeId));
        page.flowStartingPoints = final;
        figma.commitUndo();
        return {
          type: request.type,
          requestId: request.requestId,
          data: { pageId: page.id, pageName: page.name, flowStartingPoints: final },
        };
      }

      if (nodeIds.length === 0) throw new Error("At least one nodeId is required");

      const names = parseArray(p.names);
      // Resolve each target node, validate it exists, and pick its containing page.
      const entries: Array<{ nodeId: string; name: string }> = [];
      let page: any = null;
      for (let i = 0; i < nodeIds.length; i++) {
        const id = nodeIds[i];
        const node = await figma.getNodeByIdAsync(id);
        if (!node) throw new Error(`Node not found: ${id}`);
        // Prototypes (and flowStartingPoints) are per-page — resolve the owning page.
        const ancestor = owningPage(node);
        if (!ancestor) throw new Error(`Node ${id} is not on a page`);
        if (page == null) page = ancestor;
        else if (page.id !== ancestor.id) {
          throw new Error("All starting-point nodes must be on the same page");
        }
        const name = (names[i] != null && String(names[i])) || (node as any).name || `Flow ${i + 1}`;
        entries.push({ nodeId: id, name });
      }

      const current: Array<{ nodeId: string; name: string }> = page.flowStartingPoints
        ? [...page.flowStartingPoints]
        : [];
      // `append` skips entries whose nodeId is already a starting point; `replace`
      // (default) sets the page's start points to exactly the given frames. (`remove`
      // and `clear` are handled in their own tolerant branches above.)
      const final =
        mode === "append"
          ? [...current, ...entries.filter((e) => !current.some((c) => c.nodeId === e.nodeId))]
          : entries;

      page.flowStartingPoints = final;
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { pageId: page.id, pageName: page.name, flowStartingPoints: final },
      };
    }

    case "set_overflow": {
      // Prototype scroll direction on a frame. NOTE: a nested frame only scrolls in
      // presentation when its content exceeds its bounds AND clipsContent is true — set
      // clipsContent here so callers don't hit "scroll doesn't work".
      const p = request.params || {};
      const dir = p.overflowDirection;
      if (dir == null || !OVERFLOW_DIRECTIONS.includes(dir)) {
        throw new Error(`overflowDirection must be one of ${OVERFLOW_DIRECTIONS.join(", ")}`);
      }
      const nodeIds: string[] = request.nodeIds ?? [];
      if (nodeIds.length === 0) throw new Error("nodeId is required");
      // Resolve + validate every node BEFORE mutating any, so a bad node in the list
      // doesn't leave earlier nodes half-applied outside a clean undo group.
      const nodes: any[] = [];
      for (const id of nodeIds) {
        const node = await figma.getNodeByIdAsync(id);
        if (!node) throw new Error(`Node not found: ${id}`);
        if (!("overflowDirection" in node)) {
          throw new Error(`Node ${id} does not support overflowDirection (not a frame-like node)`);
        }
        if (p.clipsContent != null && !("clipsContent" in node)) {
          throw new Error(`Node ${id} does not support clipsContent`);
        }
        nodes.push(node);
      }
      const results = nodes.map((node) => {
        node.overflowDirection = dir;
        if (p.clipsContent != null) node.clipsContent = !!p.clipsContent;
        return {
          id: node.id,
          name: node.name,
          overflowDirection: node.overflowDirection,
          clipsContent: node.clipsContent,
        };
      });
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: results.length === 1 ? results[0] : { results },
      };
    }

    case "set_fixed_children": {
      // Low-level: set how many LEADING children stay fixed while the rest scroll. The
      // caller owns child order + absolute positioning (see pin_child for the convenience path).
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const n = p.numberOfFixedChildren;
      if (typeof n !== "number" || !Number.isInteger(n) || n < 0) {
        throw new Error("numberOfFixedChildren must be a non-negative integer");
      }
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (!("numberOfFixedChildren" in node)) {
        throw new Error(`Node ${nodeId} does not support numberOfFixedChildren (not a frame-like node)`);
      }
      const childCount = "children" in node ? (node as any).children.length : 0;
      if (n > childCount) {
        throw new Error(`numberOfFixedChildren (${n}) exceeds the frame's child count (${childCount})`);
      }
      (node as any).numberOfFixedChildren = n;
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: (node as any).name, numberOfFixedChildren: n },
      };
    }

    case "pin_child": {
      // High-level convenience: pin a child so it stays put while its frame scrolls.
      // Mutates the child (layoutPositioning ABSOLUTE) and its order (moved into the
      // leading fixed band), then extends the parent's fixed-children count.
      const nodeIds: string[] = request.nodeIds ?? [];
      if (nodeIds.length === 0) throw new Error("nodeId is required");
      // Resolve + validate every child (and its parent) BEFORE mutating any.
      const pairs: Array<{ child: any; parent: any }> = [];
      for (const id of nodeIds) {
        const child = await figma.getNodeByIdAsync(id);
        if (!child) throw new Error(`Node not found: ${id}`);
        const parent: any = (child as any).parent;
        if (!parent) throw new Error(`Node ${id} has no parent to pin within`);
        if (!("numberOfFixedChildren" in parent)) {
          throw new Error(`Parent of ${id} does not support fixed children (must be a frame)`);
        }
        pairs.push({ child, parent });
      }
      const results = pairs.map(({ child, parent }) => {
        if ("layoutPositioning" in child) child.layoutPositioning = "ABSOLUTE";
        // Idempotent: if the child is already in the leading fixed band, leave the order
        // and count alone (re-pinning must not over-count). Otherwise move it into the
        // band and extend the count, clamped to the child count.
        const currentIndex = parent.children ? parent.children.indexOf(child) : -1;
        const alreadyFixed = currentIndex > -1 && currentIndex < (parent.numberOfFixedChildren || 0);
        if (!alreadyFixed) {
          const insertAt = parent.numberOfFixedChildren || 0;
          parent.insertChild(insertAt, child);
          parent.numberOfFixedChildren = Math.min(insertAt + 1, parent.children.length);
        }
        return {
          id: child.id,
          name: child.name,
          parentId: parent.id,
          numberOfFixedChildren: parent.numberOfFixedChildren,
        };
      });
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: results.length === 1 ? results[0] : { results },
      };
    }

    case "set_prototype_background": {
      // Page-level prototype presentation background (single solid color). Targets the
      // owning page of the first nodeId, else the current page. mode "clear" empties it.
      const p = request.params || {};
      const nodeIds: string[] = request.nodeIds ?? [];
      const mode = p.mode || "set";
      let page: any = figma.currentPage;
      if (nodeIds.length > 0) {
        const node = await figma.getNodeByIdAsync(nodeIds[0]);
        if (!node) throw new Error(`Node not found: ${nodeIds[0]}`);
        const ancestor = owningPage(node);
        if (!ancestor) throw new Error(`Node ${nodeIds[0]} is not on a page`);
        page = ancestor;
      }
      if (mode === "clear") {
        page.prototypeBackgrounds = [];
      } else {
        if (!p.color) throw new Error('set_prototype_background requires "color" (hex) unless mode is "clear"');
        page.prototypeBackgrounds = [makeSolidPaint(p.color, p.opacity != null ? p.opacity : undefined)];
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { pageId: page.id, pageName: page.name, prototypeBackgrounds: page.prototypeBackgrounds },
      };
    }

    default:
      return null;
  }
};
