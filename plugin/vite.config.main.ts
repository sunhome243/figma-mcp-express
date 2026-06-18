import { defineConfig } from "vite";

// Plugin sandbox code (code.js). Minification is mode-gated:
//   - production (default `vite build`, incl. release CI) → minified, ~47% smaller
//     (190KB → ~101KB on the real bundle) so consumers get the optimal artifact.
//   - development (`vite build --mode development`, used by the `dev` watch) → NOT
//     minified, so plugin-logic stack traces stay readable. (Figma's sandbox has
//     unreliable sourcemap support, so readability relies on un-minified code, not maps.)
export default defineConfig(({ mode }) => ({
  define: {
    __PLUGIN_ID__: JSON.stringify("figma-mcp-express"),
    __DEFAULT_PORT__: JSON.stringify("1994"),
  },
  build: {
    target: "es2015",
    lib: {
      entry: "src/main.ts",
      formats: ["iife"],
      name: "code",
      fileName: () => "code.js",
    },
    outDir: "dist",
    emptyOutDir: false,
    minify: mode === "production" ? "esbuild" : false,
    sourcemap: mode !== "production",
  },
}));
