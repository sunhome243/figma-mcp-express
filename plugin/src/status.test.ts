import { describe, it, expect } from "bun:test";
import {
  statusEquals,
  nextReconnectDelay,
  RECONNECT_BASE_MS,
  RECONNECT_CAP_MS,
  type PluginStatus,
} from "./status";

// ── statusEquals (diff-before-send) ──────────────────────────────────────────

describe("statusEquals", () => {
  const base: PluginStatus = {
    fileName: "F",
    fileKey: "K",
    pageName: "P",
    selectionCount: 0,
  };

  it("treats null previous as not-equal (first send always fires)", () => {
    expect(statusEquals(null, base)).toBe(false);
  });

  it("is true when every field matches", () => {
    expect(statusEquals({ ...base }, { ...base })).toBe(true);
  });

  it("detects a selectionCount change (the pan-storm case)", () => {
    expect(statusEquals(base, { ...base, selectionCount: 3 })).toBe(false);
  });

  it("detects a pageName change", () => {
    expect(statusEquals(base, { ...base, pageName: "Other" })).toBe(false);
  });

  it("detects a fileName / fileKey change", () => {
    expect(statusEquals(base, { ...base, fileName: "G" })).toBe(false);
    expect(statusEquals(base, { ...base, fileKey: "K2" })).toBe(false);
  });
});

// ── nextReconnectDelay (exponential backoff + cap) ───────────────────────────

describe("nextReconnectDelay", () => {
  it("starts at the base delay on the first attempt", () => {
    expect(nextReconnectDelay(0)).toBe(RECONNECT_BASE_MS);
  });

  it("doubles each attempt until the cap", () => {
    expect(nextReconnectDelay(1)).toBe(RECONNECT_BASE_MS * 2);
    expect(nextReconnectDelay(2)).toBe(RECONNECT_BASE_MS * 4);
    expect(nextReconnectDelay(3)).toBe(RECONNECT_BASE_MS * 8);
  });

  it("never exceeds the cap", () => {
    for (let attempt = 0; attempt < 40; attempt++) {
      expect(nextReconnectDelay(attempt)).toBeLessThanOrEqual(RECONNECT_CAP_MS);
    }
    // A very large attempt count is clamped to the cap, not Infinity.
    expect(nextReconnectDelay(1000)).toBe(RECONNECT_CAP_MS);
  });

  it("is monotonic non-decreasing", () => {
    let prev = 0;
    for (let attempt = 0; attempt < 20; attempt++) {
      const d = nextReconnectDelay(attempt);
      expect(d).toBeGreaterThanOrEqual(prev);
      prev = d;
    }
  });
});
