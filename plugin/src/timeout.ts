// withTimeout — race a promise against a reject-on-timeout.
//
// The Figma Plugin API has async calls that can HANG and never resolve/reject
// (a hung importByKeyAsync, a wedged node mutation). The plugin is single-
// threaded and each tool call holds the server's per-channel serial slot until
// it resolves, so one hung op stalls every other agent on that channel until the
// server's inactivity ceiling (~120s) force-drains it — the harsh failure path.
// Wrapping a dispatch in withTimeout converts that into a graceful, actionable
// rejection at a chosen horizon, freeing the channel cleanly. The timer is always
// cleared on settle (no leak). Exported for direct unit testing with a small ms.
export const withTimeout = <T>(
  p: Promise<T>,
  label: string,
  ms: number,
  hint?: string,
): Promise<T> => {
  let timer: ReturnType<typeof setTimeout>;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      const suffix = hint ? ` — ${hint}` : "";
      reject(new Error(`${label} timed out after ${ms}ms${suffix}`));
    }, ms);
  });
  return Promise.race([p, timeout]).finally(() => clearTimeout(timer)) as Promise<T>;
};
