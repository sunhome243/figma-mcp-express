// Shared progress/heartbeat helper for long-running plugin operations.
//
// The Figma plugin sandbox is single-threaded: a long traversal, serialization,
// export, or bulk-write loop monopolises the JS thread. Two things must happen
// periodically so the operation survives:
//
//   1. Yield the event loop (`await setTimeout(0)`) so the message pump can flush
//      and other queued commands can interleave.
//   2. Post a `progress_update` to the UI → Go bridge, which RESETS the Go-side
//      per-request inactivity timeout (see internal/bridge.go). Without a tick,
//      a long op that exceeds the window (120s writes / 600s heavy reads) is
//      killed by the watchdog even though it is making progress.
//
// The returned data shape of every caller is byte-for-byte identical — yielding
// and progress messages are side-effects only.

/**
 * Returns an async tick function that posts a `progress_update` with
 * `progress > 0` (required so the Go bridge treats it as activity and resets the
 * timer) whenever EITHER cadence fires:
 *   - count-based: every `every` calls (cheap; dominates fast high-count walks);
 *   - time-based: at least once per `heartbeatMs` of wall-clock while the loop is
 *     executing (the liveness floor).
 *
 * The time-based floor matters because the count cadence is node-COUNT, not time:
 * a heavy read that is slow but low-count (few nodes, each an expensive async Figma
 * call) could otherwise run many seconds emitting ZERO progress, which would (a)
 * risk the Go inactivity ceiling and (b) trip the stalled-head guard
 * (internal/bridge.go) into falsely rejecting a peer. Ticking on elapsed time keeps
 * a *progressing* op's liveness fresh; a genuinely hung op (stuck in a single await,
 * so it never calls tick) still emits nothing and is correctly detected. The
 * returned data shape is unchanged — ticks are side-effects only.
 *
 * @param requestId    the in-flight request id the bridge keys the timer on
 * @param label        human label for the progress message
 * @param every        count cadence — default 800 (node walks); pass 1 for
 *                     per-item loops like exports where each item is expensive
 * @param heartbeatMs  wall-clock liveness floor — emit at least this often while
 *                     the loop runs (default 10s, well under the 45s stall guard)
 */
export function makeProgress(
  requestId: string,
  label: string,
  every = 800,
  heartbeatMs = 10_000,
) {
  let n = 0;
  let lastTickAt = Date.now();
  return async (total?: number) => {
    n++;
    const now = Date.now();
    if (n % every === 0 || now - lastTickAt >= heartbeatMs) {
      lastTickAt = now;
      await new Promise<void>((r) => setTimeout(r, 0));
      figma.ui.postMessage({
        type: "progress_update",
        requestId,
        progress: total ? Math.min(99, Math.round((n / total) * 99)) : 1,
        message: `${label}: processed ${n} nodes`,
      });
    }
  };
}
