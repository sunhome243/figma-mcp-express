import { hexToRgb } from "./write-helpers";

const requireStringField = (field: string, value: unknown): string => {
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`${field} must be a string`);
  }
  return value;
};

const requireBooleanField = (field: string, value: unknown): boolean => {
  if (typeof value !== "boolean") {
    throw new Error(`${field} must be a boolean`);
  }
  return value;
};

const VALID_CODE_SYNTAX_PLATFORMS = new Set(["WEB", "ANDROID", "iOS"]);
const VALID_VARIABLE_SCOPES = new Set([
  "ALL_SCOPES",
  "TEXT_CONTENT",
  "CORNER_RADIUS",
  "WIDTH_HEIGHT",
  "GAP",
  "ALL_FILLS",
  "FRAME_FILL",
  "SHAPE_FILL",
  "TEXT_FILL",
  "STROKE_COLOR",
  "STROKE_FLOAT",
  "EFFECT_FLOAT",
  "EFFECT_COLOR",
  "OPACITY",
  "FONT_FAMILY",
  "FONT_STYLE",
  "FONT_WEIGHT",
  "FONT_SIZE",
  "LINE_HEIGHT",
  "LETTER_SPACING",
  "PARAGRAPH_SPACING",
  "PARAGRAPH_INDENT",
]);

const requireCodeSyntaxPlatform = (platform: unknown): CodeSyntaxPlatform => {
  const value = requireStringField("codeSyntax platform", platform);
  if (!VALID_CODE_SYNTAX_PLATFORMS.has(value)) {
    throw new Error(`codeSyntax platform must be WEB, ANDROID, or iOS, got: ${value}`);
  }
  return value as CodeSyntaxPlatform;
};

const validateScopes = (value: unknown): VariableScope[] => {
  if (!Array.isArray(value)) {
    throw new Error("scopes must be an array");
  }
  return value.map((scope) => {
    const text = requireStringField("scopes[]", scope);
    if (!VALID_VARIABLE_SCOPES.has(text)) {
      throw new Error(`invalid variable scope: "${text}"`);
    }
    return text as VariableScope;
  });
};

const validateCodeSyntaxUpdates = (value: unknown): Array<[CodeSyntaxPlatform, string]> => {
  if (value == null) return [];
  if (typeof value !== "object" || Array.isArray(value)) {
    throw new Error("codeSyntax must be an object");
  }
  return Object.entries(value as Record<string, unknown>).map(([platform, syntax]) => [
    requireCodeSyntaxPlatform(platform),
    requireStringField(`codeSyntax.${platform}`, syntax),
  ]);
};

const validateCodeSyntaxRemovals = (value: unknown): CodeSyntaxPlatform[] => {
  if (value == null) return [];
  if (!Array.isArray(value)) {
    throw new Error("removeCodeSyntax must be an array");
  }
  return value.map((platform) => requireCodeSyntaxPlatform(platform));
};

const parseVariableValue = (type: string, value: any): VariableValue => {
  // VARIABLE_ALIAS must be passed through unchanged before any type coercion: the other
  // branches would corrupt it (FLOAT → NaN, STRING → "[object Object]", BOOLEAN → false).
  if (value && typeof value === "object" && value.type === "VARIABLE_ALIAS") {
    return { type: "VARIABLE_ALIAS", id: value.id } as VariableValue;
  }
  if (type === "COLOR") {
    if (typeof value === "string") {
      const { r, g, b, a } = hexToRgb(value);
      return { r, g, b, a };
    }
    return value as RGBA;
  }
  if (type === "FLOAT") return typeof value === "number" ? value : parseFloat(String(value));
  if (type === "BOOLEAN") return value === true || value === "true";
  return String(value); // STRING
};

export const handleWriteVariableRequest = async (request: any) => {
  switch (request.type) {
    case "create_variable_collection": {
      const p = request.params || {};
      if (!p.name) throw new Error("name is required");
      const collection = figma.variables.createVariableCollection(p.name);
      if (p.initialModeName && collection.modes.length > 0) {
        collection.renameMode(collection.modes[0].modeId, p.initialModeName);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: collection.id,
          name: collection.name,
          modes: collection.modes.map((m) => ({ modeId: m.modeId, name: m.name })),
        },
      };
    }

    case "add_variable_mode": {
      const p = request.params || {};
      if (!p.collectionId) throw new Error("collectionId is required");
      if (!p.modeName) throw new Error("modeName is required");
      const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
      if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
      let modeId: string;
      try {
        modeId = collection.addMode(p.modeName);
      } catch (err: unknown) {
        const origMessage = err instanceof Error ? err.message : String(err);
        throw new Error(`Cannot add mode (plan mode-limit reached or collection is read-only): ${origMessage}`);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { collectionId: p.collectionId, modeId, modeName: p.modeName },
      };
    }

    case "create_variable": {
      const p = request.params || {};
      if (!p.name) throw new Error("name is required");
      if (!p.collectionId) throw new Error("collectionId is required");
      const validTypes = ["COLOR", "FLOAT", "STRING", "BOOLEAN"];
      if (!p.type || !validTypes.includes(p.type)) {
        throw new Error("type is required: COLOR, FLOAT, STRING, or BOOLEAN");
      }
      const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
      if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
      const variable = figma.variables.createVariable(p.name, collection, p.type as VariableResolvedDataType);
      // Batch-set values across multiple modes when p.values is a {modeId: value} map;
      // the p.value shorthand (modes[0] only) is the fallback for single-mode use.
      if (p.values != null && typeof p.values === "object") {
        for (const [modeId, val] of Object.entries(p.values)) {
          variable.setValueForMode(modeId, parseVariableValue(p.type, val));
        }
      } else if (p.value != null && collection.modes.length > 0) {
        // p.value is the shorthand for modes[0]
        const modeId = collection.modes[0].modeId;
        variable.setValueForMode(modeId, parseVariableValue(p.type, p.value));
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: variable.id,
          name: variable.name,
          resolvedType: variable.resolvedType,
          collectionId: p.collectionId,
          // Return the collection's mode list so callers have the modeIds they need
          // to call set_variable_value without a separate collection-lookup round-trip.
          modes: collection.modes.map((m: { modeId: string; name: string }) => ({ modeId: m.modeId, name: m.name })),
        },
      };
    }

    case "set_variable_value": {
      const p = request.params || {};
      if (!p.variableId) throw new Error("variableId is required");
      if (!p.modeId) throw new Error("modeId is required");
      if (p.value == null) throw new Error("value is required");
      const variable = await figma.variables.getVariableByIdAsync(p.variableId);
      if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
      // Validate modeId against the variable's collection before writing — Figma silently
      // creates a new (orphaned) mode entry when given an unknown modeId.
      const coll = await figma.variables.getVariableCollectionByIdAsync(variable.variableCollectionId);
      if (coll && !coll.modes.some((m: { modeId: string }) => m.modeId === p.modeId)) {
        const validIds = coll.modes.map((m: { modeId: string }) => m.modeId).join(", ");
        throw new Error(`modeId "${p.modeId}" not found in collection "${coll.name}". Valid modeIds: ${validIds}`);
      }
      variable.setValueForMode(p.modeId, parseVariableValue(variable.resolvedType, p.value));
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { variableId: variable.id, name: variable.name, modeId: p.modeId },
      };
    }

    case "create_variable_alias": {
      const p = request.params || {};
      if (!p.variableId) throw new Error("variableId is required");
      const alias = await figma.variables.createVariableAliasByIdAsync(String(p.variableId));
      return {
        type: request.type,
        requestId: request.requestId,
        data: { alias },
      };
    }

    case "delete_variable": {
      const p = request.params || {};
      if (p.variableId) {
        const variable = await figma.variables.getVariableByIdAsync(p.variableId);
        if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
        variable.remove();
        figma.commitUndo();
        return {
          type: request.type,
          requestId: request.requestId,
          data: { variableId: p.variableId, deleted: true },
        };
      } else if (p.collectionId) {
        const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
        if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
        collection.remove();
        figma.commitUndo();
        return {
          type: request.type,
          requestId: request.requestId,
          data: { collectionId: p.collectionId, deleted: true },
        };
      } else {
        throw new Error("variableId or collectionId is required");
      }
    }

    case "update_variable": {
      const p = request.params || {};
      if (!p.variableId) throw new Error("variableId is required");
      const variable = await figma.variables.getVariableByIdAsync(p.variableId);
      if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
      const nextName = p.name != null ? requireStringField("name", p.name) : undefined;
      const nextScopes = p.scopes != null ? validateScopes(p.scopes) : undefined;
      const nextHidden = p.hiddenFromPublishing != null
        ? requireBooleanField("hiddenFromPublishing", p.hiddenFromPublishing)
        : undefined;
      const codeSyntaxUpdates = validateCodeSyntaxUpdates(p.codeSyntax);
      const codeSyntaxRemovals = validateCodeSyntaxRemovals(p.removeCodeSyntax);
      if (nextName !== undefined) variable.name = nextName;
      if (nextScopes !== undefined) variable.scopes = nextScopes;
      if (nextHidden !== undefined) variable.hiddenFromPublishing = nextHidden;
      for (const [platform, value] of codeSyntaxUpdates) {
        variable.setVariableCodeSyntax(platform, value);
      }
      for (const platform of codeSyntaxRemovals) {
        variable.removeVariableCodeSyntax(platform);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          variableId: variable.id,
          name: variable.name,
          scopes: variable.scopes,
          hiddenFromPublishing: variable.hiddenFromPublishing,
          codeSyntax: variable.codeSyntax,
        },
      };
    }

    case "update_variable_collection": {
      const p = request.params || {};
      if (!p.collectionId) throw new Error("collectionId is required");
      const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
      if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
      const nextName = p.name != null ? requireStringField("name", p.name) : undefined;
      const nextHidden = p.hiddenFromPublishing != null
        ? requireBooleanField("hiddenFromPublishing", p.hiddenFromPublishing)
        : undefined;
      let renameMode: { modeId: string; newName: string } | undefined;
      if (p.renameMode != null) {
        if (typeof p.renameMode !== "object" || Array.isArray(p.renameMode)) {
          throw new Error("renameMode must be an object");
        }
        if (!p.renameMode.modeId || p.renameMode.newName == null) {
          throw new Error("renameMode requires { modeId, newName }");
        }
        const modeId = requireStringField("renameMode.modeId", p.renameMode.modeId);
        const newName = requireStringField("renameMode.newName", p.renameMode.newName);
        if (!collection.modes.some((m: { modeId: string }) => m.modeId === modeId)) {
          throw new Error(`Mode not found: ${modeId}`);
        }
        renameMode = { modeId, newName };
      }
      let removeMode: string | undefined;
      if (p.removeMode != null) {
        removeMode = requireStringField("removeMode", p.removeMode);
        if (!collection.modes.some((m: { modeId: string }) => m.modeId === removeMode)) {
          throw new Error(`Mode not found: ${removeMode}`);
        }
        if (collection.modes.length <= 1) {
          throw new Error("Cannot remove mode (a collection must keep at least one mode)");
        }
      }
      if (nextName !== undefined) collection.name = nextName;
      if (nextHidden !== undefined) collection.hiddenFromPublishing = nextHidden;
      if (renameMode) {
        collection.renameMode(renameMode.modeId, renameMode.newName);
      }
      if (removeMode != null) {
        // Figma throws if you remove the last remaining mode — surface a clear message.
        try {
          collection.removeMode(removeMode);
        } catch (err: unknown) {
          const origMessage = err instanceof Error ? err.message : String(err);
          throw new Error(`Cannot remove mode (a collection must keep at least one mode): ${origMessage}`);
        }
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          collectionId: collection.id,
          name: collection.name,
          hiddenFromPublishing: collection.hiddenFromPublishing,
          modes: collection.modes.map((m) => ({ modeId: m.modeId, name: m.name })),
        },
      };
    }

    default:
      return null;
  }
};
