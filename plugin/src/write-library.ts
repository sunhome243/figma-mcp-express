// Library handlers — Track A: component/variable/style import, instancing,
// instance property application, and variable-mode pinning.

import { bulkApply } from "./write-helpers";

const importComponentOrSet = async (key: string, assetType?: string) => {
  if (assetType === "COMPONENT_SET") {
    return await figma.importComponentSetByKeyAsync(key);
  }
  try {
    return await figma.importComponentByKeyAsync(key);
  } catch (e: any) {
    // Only fall back to COMPONENT_SET import when the error indicates a type
    // mismatch or a simple "not found" — i.e. the key may belong to a set.
    // Any other error (library disabled, network, permission) is re-thrown as-is
    // to avoid masking the real root cause.
    const msg: string = (e?.message ?? "").toLowerCase();
    if (msg.includes("not found") || msg.includes("not a component")) {
      return await figma.importComponentSetByKeyAsync(key);
    }
    throw e;
  }
};

// loadInstanceFonts — loads all unique fonts from TEXT descendants of an
// instance before setProperties is called. Skips figma.mixed fontNames (e.g.
// nodes with multiple fonts applied) to avoid passing a Symbol to loadFontAsync.
const loadInstanceFonts = async (inst: any): Promise<void> => {
  const textNodes: any[] = inst.findAll ? inst.findAll((n: any) => n.type === "TEXT") : [];
  const seen = new Set<string>();
  for (const n of textNodes) {
    const fn = n.fontName;
    if (!fn || fn === figma.mixed) continue;
    const key = `${fn.family}::${fn.style}`;
    if (!seen.has(key)) {
      seen.add(key);
      await figma.loadFontAsync(fn);
    }
  }
};

// setPropertiesSafe — drops SLOT-type keys (Figma's setProperties cannot set
// slot properties; passing them throws "cannotSetSlotProperty" and poisons the
// whole update). Returns the dropped keys so callers can report them.
const setPropertiesSafe = async (
  node: any,
  properties: Record<string, any>,
): Promise<{ dropped: string[] }> => {
  const componentProps: Record<string, { type: string }> = node.componentProperties ?? {};
  const dropped: string[] = [];
  const filtered: Record<string, any> = {};
  for (const [k, v] of Object.entries(properties)) {
    if (componentProps[k]?.type === "SLOT") {
      dropped.push(k);
    } else {
      filtered[k] = v;
    }
  }
  await loadInstanceFonts(node);
  if (Object.keys(filtered).length > 0) {
    node.setProperties(filtered);
  }
  return { dropped };
};

const selectVariant = (node: any, variantProperties?: Record<string, any>) => {
  if (node.type !== "COMPONENT_SET") return node;
  const variants: any[] = node.children || [];
  if (variantProperties) {
    const match = variants.find((c: any) =>
      c.variantProperties &&
      Object.entries(variantProperties).every(([k, v]) => c.variantProperties[k] === v),
    );
    if (!match) {
      throw new Error(`No variant matches properties: ${JSON.stringify(variantProperties)}`);
    }
    return match;
  }
  return node.defaultVariant;
};

export const handleWriteLibraryRequest = async (request: any) => {
  switch (request.type) {
    // ── MUTATING ──────────────────────────────────────────────────────────────

    case "create_instance": {
      const p = request.params || {};
      if (!p.componentId) throw new Error("componentId is required");
      let component: any = await figma.getNodeByIdAsync(p.componentId);
      if (!component && p.componentKey) {
        component = await importComponentOrSet(p.componentKey);
      }
      if (!component) throw new Error(`Component not found: ${p.componentId}`);
      const chosen = selectVariant(component, p.variantProperties);
      const inst = chosen.createInstance();

      const parent = p.parentId
        ? await figma.getNodeByIdAsync(p.parentId)
        : figma.currentPage;
      if (!parent) throw new Error(`Parent not found: ${p.parentId}`);
      if (p.index != null) {
        (parent as any).insertChild(p.index, inst);
      } else {
        (parent as any).appendChild(inst);
      }

      if (p.x != null) inst.x = p.x;
      if (p.y != null) inst.y = p.y;
      if (p.width != null || p.height != null) {
        inst.resize(p.width != null ? p.width : inst.width, p.height != null ? p.height : inst.height);
      }
      if (p.layoutSizingHorizontal != null) inst.layoutSizingHorizontal = p.layoutSizingHorizontal;
      if (p.layoutSizingVertical != null) inst.layoutSizingVertical = p.layoutSizingVertical;
      // setProperties can't set SLOT props (throws cannotSetSlotProperty) and needs fonts
      // loaded for TEXT props; setPropertiesSafe handles both and reports which SLOT keys it
      // had to drop. Surface them so a caller passing a SLOT at creation isn't left guessing
      // why it had no effect (mirrors set_instance_properties' droppedSlotKeys).
      const dropped = p.properties ? (await setPropertiesSafe(inst, p.properties)).dropped : [];

      figma.commitUndo();
      const data: any = { id: inst.id, name: inst.name };
      if (dropped.length > 0) data.droppedSlotKeys = dropped;
      return {
        type: request.type,
        requestId: request.requestId,
        data,
      };
    }

    case "set_instance_properties": {
      const p = request.params || {};
      // Request-level throw: properties required for the whole op. Per-node
      // (wrong type / missing) collected so a bulk apply over many instances
      // doesn't abort on one bad id.
      if ((request.nodeIds || []).length === 0) throw new Error("nodeIds is required");
      if (!p.properties) throw new Error("properties is required");
      return bulkApply(request, async (node, nid) => {
        if (node.type !== "INSTANCE") throw new Error(`Node ${nid} is not a component INSTANCE`);
        if (p.resetOverrides) node.resetOverrides();
        // Use setPropertiesSafe: loads fonts first, drops SLOT-type keys to avoid
        // "cannotSetSlotProperty" errors that would poison the whole node update.
        const { dropped } = await setPropertiesSafe(node, p.properties);
        const result: any = { name: node.name, appliedProperties: p.properties };
        if (dropped.length > 0) result.droppedSlotKeys = dropped;
        return result;
      });
    }

    case "set_variable_mode": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      if (!p.collectionId) throw new Error("collectionId is required");
      if (!p.modeId) throw new Error("modeId is required");
      const node = await figma.getNodeByIdAsync(nodeId) as any;
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
      if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
      node.setExplicitVariableModeForCollection(collection, p.modeId);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: node.name, collectionId: p.collectionId, modeId: p.modeId },
      };
    }

    // ── NON-MUTATING ────────────────────────────────────────────────────────────

    case "import_component_by_key": {
      const p = request.params || {};
      if (!p.key) throw new Error("key is required");
      const result: any = await importComponentOrSet(p.key, p.assetType);
      if (result.type === "COMPONENT_SET") {
        return {
          type: request.type,
          requestId: request.requestId,
          data: {
            id: result.id,
            name: result.name,
            type: "COMPONENT_SET",
            defaultVariantId: result.defaultVariant.id,
            variants: (result.children as any[]).map((c: any) => ({
              id: c.id,
              name: c.name,
              variantProperties: c.variantProperties,
            })),
          },
        };
      }
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: result.id, name: result.name, type: "COMPONENT" },
      };
    }

    case "import_variable_by_key": {
      const p = request.params || {};
      if (!p.key) throw new Error("key is required");
      // Unlike the component/style import calls (top-level on figma), variable
      // import lives on figma.variables (plugin-typings line 2181).
      const variable: any = await figma.variables.importVariableByKeyAsync(p.key);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: variable.id, name: variable.name, resolvedType: variable.resolvedType },
      };
    }

    case "import_style_by_key": {
      const p = request.params || {};
      if (!p.key) throw new Error("key is required");
      const style: any = await figma.importStyleByKeyAsync(p.key);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: style.id, name: style.name, styleType: style.type },
      };
    }

    case "get_remote_variable_collection": {
      const p = request.params || {};
      if (!p.collectionId) throw new Error("collectionId is required");
      const collection: any = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
      if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: collection.id,
          name: collection.name,
          defaultModeId: collection.defaultModeId,
          modes: collection.modes,
        },
      };
    }

    case "list_library_variable_collections": {
      const collections = await figma.teamLibrary.getAvailableLibraryVariableCollectionsAsync();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { collections },
      };
    }

    case "get_library_variables": {
      const key = request.params?.key as string;
      if (!key) {
        return {
          type: request.type,
          requestId: request.requestId,
          error: "key is required",
        };
      }
      const variables = await figma.teamLibrary.getVariablesInLibraryCollectionAsync(key);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { variables },
      };
    }

    default:
      return null;
  }
};
