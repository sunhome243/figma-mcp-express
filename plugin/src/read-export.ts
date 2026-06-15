import { makeProgress } from "./progress";

export const handleReadExportRequest = async (request: any) => {
  switch (request.type) {
    case "get_screenshot": {
      const format =
        request.params && request.params.format
          ? request.params.format
          : "PNG";
      const scale =
        request.params && request.params.scale != null
          ? request.params.scale
          : 2;
      let targetNodes: any[];
      if (request.nodeIds && request.nodeIds.length > 0) {
        const nodes = await Promise.all(
          request.nodeIds.map((id: string) => figma.getNodeByIdAsync(id)),
        );
        targetNodes = nodes.filter(
          (n) => n !== null && n.type !== "DOCUMENT" && n.type !== "PAGE",
        );
      } else {
        targetNodes = figma.currentPage.selection.slice();
      }
      if (targetNodes.length === 0)
        throw new Error(
          "No nodes to export. Select nodes or provide nodeIds.",
        );
      // Export serially with a per-frame heartbeat. exportAsync is CPU-bound and
      // blocks the single JS thread; a parallel Promise.all over many frames runs
      // them back-to-back with NO progress tick in between, so a large export set
      // can exceed the Go-bridge inactivity window and be killed mid-flight (see
      // feedback_concurrent-screenshots-jam-serial). tick(every=1) yields + posts
      // a progress_update after each frame, resetting the timer.
      const tick = makeProgress(request.requestId, "get_screenshot", 1);
      const exports: any[] = [];
      for (const node of targetNodes) {
        const settings: any =
          format === "SVG"
            ? { format: "SVG" }
            : format === "PDF"
              ? { format: "PDF" }
              : format === "JPG"
                ? { format: "JPG", constraint: { type: "SCALE", value: scale } }
                : { format: "PNG", constraint: { type: "SCALE", value: scale } };
        const bytes = await node.exportAsync(settings);
        const base64 = figma.base64Encode(bytes);
        exports.push({
          nodeId: node.id,
          nodeName: node.name,
          format,
          base64,
          width: node.width,
          height: node.height,
        });
        await tick(targetNodes.length);
      }
      return {
        type: request.type,
        requestId: request.requestId,
        data: { exports },
      };
    }

    case "export_frames_to_pdf": {
      const nodeIds: string[] = request.nodeIds ?? [];
      if (nodeIds.length === 0) {
        throw new Error("nodeIds is required and must not be empty");
      }
      const tick = makeProgress(request.requestId, "export_frames_to_pdf", 1);
      const frames: any[] = [];
      for (const id of nodeIds) {
        const node = await figma.getNodeByIdAsync(id);
        if (!node || node.type === "DOCUMENT" || node.type === "PAGE") {
          throw new Error(`Node ${id} not found or is not exportable`);
        }
        const bytes = await (node as any).exportAsync({ format: "PDF" });
        const base64 = figma.base64Encode(bytes);
        frames.push({
          nodeId: node.id,
          nodeName: node.name,
          base64,
        });
        await tick(nodeIds.length);
      }
      return {
        type: request.type,
        requestId: request.requestId,
        data: { frames },
      };
    }

    default:
      return null;
  }
};
