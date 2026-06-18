import { describe, it, expect } from "bun:test";
// The mode-gated minify toggle: consumers (production build, incl. release CI) get a
// minified bundle; the dev watch (--mode development) stays unminified for readable
// stack traces. We test the config factory directly — fast, no full vite build — and
// the real ~47% size win (190KB → ~101KB) was measured separately on the bundle.
import mainConfig from "../vite.config.main";

type ConfigFn = (env: { mode: string; command: "build" | "serve" }) => any;

async function resolve(mode: string) {
  const fn = mainConfig as unknown as ConfigFn;
  return await fn({ mode, command: "build" });
}

describe("vite.config.main — mode-gated minify", () => {
  it("minifies in production (consumer/release artifact)", async () => {
    const cfg = await resolve("production");
    expect(cfg.build.minify).toBe("esbuild");
    expect(cfg.build.sourcemap).toBe(false);
  });

  it("does NOT minify in development (readable plugin-logic traces)", async () => {
    const cfg = await resolve("development");
    expect(cfg.build.minify).toBe(false);
    expect(cfg.build.sourcemap).toBe(true);
  });
});
