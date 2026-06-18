import { describe, it, expect } from "bun:test";
import { withTimeout } from "./timeout";

describe("withTimeout", () => {
  it("passes through a value that resolves before the timeout", async () => {
    const r = await withTimeout(Promise.resolve(42), "fast", 1000);
    expect(r).toBe(42);
  });

  it("propagates the underlying rejection when it loses to no timeout", async () => {
    await expect(
      withTimeout(Promise.reject(new Error("boom")), "rejecting", 1000),
    ).rejects.toThrow("boom");
  });

  it("rejects with a labeled timeout error when the promise hangs", async () => {
    await expect(
      withTimeout(new Promise(() => {}), "hung op", 20),
    ).rejects.toThrow(/hung op timed out after 20ms/);
  });

  it("appends the hint to the timeout message when provided", async () => {
    await expect(
      withTimeout(new Promise(() => {}), "op", 20, "the API hung"),
    ).rejects.toThrow(/timed out after 20ms — the API hung/);
  });
});
