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
 * Returns an async tick function that, every `every` calls, yields the JS event
 * loop and posts a `progress_update` with `progress > 0` (required so the Go
 * bridge treats it as activity and resets the timer).
 *
 * @param requestId  the in-flight request id the bridge keys the timer on
 * @param label      human label for the progress message
 * @param every      tick cadence — default 800 (node walks); pass 1 for
 *                   per-item loops like exports where each item is expensive
 */
export function makeProgress(requestId: string, label: string, every = 800) {
  let n = 0;
  return async (total?: number) => {
    n++;
    if (n % every === 0) {
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
