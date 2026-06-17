/// <reference types="@figma/plugin-typings" />

// Build-time constants injected by vite `define` (see vite.config*.ts). They let
// the production build and the isolated PoC build (poc/manifest.json) share one
// source tree: production → "figma-mcp-express"/"1994", PoC → "figma-mcp-express-poc"/"1995".
declare const __PLUGIN_ID__: string;
declare const __DEFAULT_PORT__: string;
