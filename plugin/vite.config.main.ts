import { defineConfig } from "vite";

// POC=1 builds the plugin core into poc/dist/ with the PoC default port so the
// isolated PoC plugin connects to its own server (1995) by default. Default
// (no env) is the unchanged production build into dist/.
const isPoc = process.env.POC === "1";

export default defineConfig({
  define: {
    __PLUGIN_ID__: JSON.stringify(isPoc ? "figma-mcp-express-poc" : "figma-mcp-express"),
    __DEFAULT_PORT__: JSON.stringify(isPoc ? "1995" : "1994"),
  },
  build: {
    target: "es2015",
    lib: {
      entry: "src/main.ts",
      formats: ["iife"],
      name: "code",
      fileName: () => "code.js",
    },
    outDir: isPoc ? "poc/dist" : "dist",
    emptyOutDir: false,
    minify: false,
  },
});
