import { describe, it, expect, beforeEach } from "bun:test";
import { makeProgress } from "./progress";

// Unit tests for the shared heartbeat helper. Every read/write/export path relies
// on these invariants: tick at the right cadence, never resolve (only progress),
// progress strictly > 0, and the message shape the Go bridge keys on.

let posted: any[];
beforeEach(() => {
  posted = [];
  (globalThis as any).figma = {
    ui: { postMessage: (m: any) => posted.push(m) },
  };
});

const ticks = (rid: string) =>
  posted.filter((m) => m.type === "progress_update" && m.requestId === rid);

describe("makeProgress — cadence", () => {
  it("default cadence 800: fires once at exactly 800 calls, zero before", async () => {
    const tick = makeProgress("r1", "get_node");
    for (let i = 0; i < 799; i++) await tick();
    expect(ticks("r1").length).toBe(0);
    await tick(); // 800th
    expect(ticks("r1").length).toBe(1);
  });

  it("cadence 50: one tick per 50 calls", async () => {
    const tick = makeProgress("r2", "set_fills", 50);
    for (let i = 0; i < 100; i++) await tick();
    expect(ticks("r2").length).toBe(2);
    for (let i = 0; i < 49; i++) await tick(); // 149 total → still 2
    expect(ticks("r2").length).toBe(2);
    await tick(); // 150th → 3
    expect(ticks("r2").length).toBe(3);
  });

  it("cadence 1: a tick on every call (export path)", async () => {
    const tick = makeProgress("r3", "get_screenshot", 1);
    await tick();
    await tick();
    await tick();
    expect(ticks("r3").length).toBe(3);
  });

  it("time-based heartbeat: emits below the count cadence once heartbeatMs elapses", async () => {
    // every=800 (so count cadence never fires here), heartbeatMs=20.
    const tick = makeProgress("r7", "get_document", 800, 20);
    await tick(); // n=1, no count tick, time not yet elapsed
    expect(ticks("r7").length).toBe(0);
    await new Promise((r) => setTimeout(r, 30)); // exceed the 20ms floor
    await tick(); // n=2, time-due → one tick despite being far below 800
    expect(ticks("r7").length).toBe(1);
  });

  it("a fast high-count loop is unaffected by the heartbeat (count cadence dominates)", async () => {
    // Default 10s heartbeat never fires in a sub-ms loop; only the 800-count fires.
    const tick = makeProgress("r8", "get_node");
    for (let i = 0; i < 800; i++) await tick();
    expect(ticks("r8").length).toBe(1);
  });
});

describe("makeProgress — message shape", () => {
  it("emits type=progress_update with the requestId and a 'processed N nodes' message", async () => {
    const tick = makeProgress("req-xyz", "get_fonts", 1);
    await tick();
    const m = ticks("req-xyz")[0];
    expect(m.type).toBe("progress_update");
    expect(m.requestId).toBe("req-xyz");
    expect(m.message).toBe("get_fonts: processed 1 nodes");
  });

  it("progress is always > 0 (Go bridge ignores progress:0 ticks)", async () => {
    const tick = makeProgress("r4", "x", 1);
    for (let i = 0; i < 5; i++) await tick();
    expect(ticks("r4").length).toBe(5);
    expect(ticks("r4").every((m) => m.progress > 0)).toBe(true);
  });

  it("with a total, progress scales toward but never reaches 100 (clamped ≤ 99)", async () => {
    const tick = makeProgress("r5", "get_screenshot", 1);
    // total=1 → n/total*99 = 99 at first tick; must stay ≤ 99, never 100.
    await tick(1);
    const m = ticks("r5")[0];
    expect(m.progress).toBeGreaterThan(0);
    expect(m.progress).toBeLessThanOrEqual(99);
  });

  it("without a total, falls back to progress=1 (still > 0)", async () => {
    const tick = makeProgress("r6", "get_document"); // cadence 800
    for (let i = 0; i < 800; i++) await tick(); // no total passed
    const m = ticks("r6")[0];
    expect(m.progress).toBe(1);
  });
});

describe("makeProgress — isolation", () => {
  it("two tickers keep independent counters (per-request, never global)", async () => {
    const a = makeProgress("ra", "a", 1);
    const b = makeProgress("rb", "b", 1);
    await a();
    await b();
    await a();
    expect(ticks("ra").length).toBe(2);
    expect(ticks("rb").length).toBe(1);
  });
});
