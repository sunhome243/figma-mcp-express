import { handleReadDocumentRequest } from "./read-document";
import { handleReadStyleRequest } from "./read-styles";
import { handleReadExportRequest } from "./read-export";
import { handleReadPrototypeRequest } from "./read-prototype";

export const handleReadRequest = async (request: any) =>
  (await handleReadDocumentRequest(request)) ??
  (await handleReadStyleRequest(request)) ??
  (await handleReadExportRequest(request)) ??
  (await handleReadPrototypeRequest(request));
