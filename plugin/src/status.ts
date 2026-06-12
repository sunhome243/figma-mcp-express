// Status diffing + reconnect backoff — pure helpers extracted so they are unit
// testable under bun:test (the plugin core `main.ts` and Svelte `App.svelte`
// can't run there, but these pure functions carry the actual logic).

// The minimal identity the plugin reports to the UI / server. Steady-state
// canvas interaction (pan/drag) never changes any of these fields, so a
// diff-before-send on this shape collapses a pan storm to zero sends.
export interface PluginStatus {
  fileName: string;
  fileKey: string;
  pageName: string;
  selectionCount: number;
}

// True when `next` is field-for-field equal to `prev`. A null `prev` (nothing
// sent yet) is never equal, so the first status always fires.
export const statusEquals = (
  prev: PluginStatus | null,
  next: PluginStatus,
): boolean =>
  prev !== null &&
  prev.fileName === next.fileName &&
  prev.fileKey === next.fileKey &&
  prev.pageName === next.pageName &&
  prev.selectionCount === next.selectionCount;

// Reconnect backoff: a server that is down should not be hit on a fixed
// metronome. Exponential from a small base, capped so it keeps retrying at a
// steady (slow) cadence rather than drifting to infinity.
export const RECONNECT_BASE_MS = 1000;
export const RECONNECT_CAP_MS = 30000;

// Delay before reconnect attempt `attempt` (0-based). base * 2^attempt, capped.
export const nextReconnectDelay = (attempt: number): number =>
  Math.min(RECONNECT_CAP_MS, RECONNECT_BASE_MS * 2 ** Math.max(0, attempt));
