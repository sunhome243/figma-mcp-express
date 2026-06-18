import { describe, it, expect } from "bun:test";
import { metaFor, avatarFor } from "./presence-roster";

// avatarFor seeds the face by (sessionId, origin) so the same truthful roster name
// in two different sessions renders a distinct — but stable — face. Pure visual
// "these are different agents" signal; the NAME stays truthful (no log mismatch).
describe("avatarFor", () => {
  it("renders a different face for the same name in two sessions", () => {
    expect(avatarFor("sessA", "grace")).not.toBe(avatarFor("sessB", "grace"));
  });

  it("is stable for the same (sessionId, origin)", () => {
    expect(avatarFor("sessA", "grace")).toBe(avatarFor("sessA", "grace"));
  });

  it("falls back to the canonical per-name face when there is no sessionId", () => {
    expect(avatarFor("", "grace")).toBe(metaFor("grace").avatar);
  });
});
