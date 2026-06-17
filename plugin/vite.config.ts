import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { viteSingleFile } from "vite-plugin-singlefile";

// POC=1 builds the isolated PoC plugin (poc/manifest.json) into poc/dist/ with a
// distinct plugin id + default port so it coexists with production without touching
// dist/. Default (no env) is the unchanged production build.
const isPoc = process.env.POC === "1";

export default defineConfig({
  plugins: [svelte(), viteSingleFile()],
  root: "./src/ui",
  define: {
    __PLUGIN_ID__: JSON.stringify(isPoc ? "figma-mcp-express-poc" : "figma-mcp-express"),
    __DEFAULT_PORT__: JSON.stringify(isPoc ? "1995" : "1994"),
  },
  build: {
    target: "es2015",
    cssCodeSplit: false,
    outDir: isPoc ? "../../poc/dist" : "../../dist",
    rollupOptions: {
      output: {
        inlineDynamicImports: true,
      },
    },
    emptyOutDir: true,
  },
});
