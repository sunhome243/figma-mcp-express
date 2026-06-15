// ESLint flat config — Figma-plugin API hygiene only.
//
// Scope is deliberately narrow: this is NOT a general style linter. It runs only
// @figma/eslint-plugin-figma-plugins, whose rules mechanically catch deprecated
// synchronous Figma APIs that are forbidden under `documentAccess: "dynamic-page"`
// (see manifest.json). This converts our "async-APIs-only" guarantee from
// hand-reviewed to enforced. The rules are type-aware, so the TS project service
// is enabled.
import figma from "@figma/eslint-plugin-figma-plugins";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    // Build output, deps, and Svelte UI (no Figma document API there) are out of scope.
    ignores: ["dist/**", "node_modules/**", "**/*.svelte"],
  },
  {
    files: ["src/**/*.ts"],
    languageOptions: {
      parser: tseslint.parser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: { "@figma/figma-plugins": figma },
    rules: figma.configs.recommended.rules,
  },
);
