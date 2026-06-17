<script lang="ts">
  import { onDestroy } from "svelte";

  // Teletypes `text` on change: backspaces to the longest common prefix with the
  // new target, then types the rest forward (~35ms/char, cancellable). Respects
  // prefers-reduced-motion (sets instantly). Self-contained — no external deps.
  export let text: string = "";

  const STEP_MS = 35;

  const reduceMotion =
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  let shown = text; // currently displayed string
  let target = text; // string we're animating toward
  let animating = false;
  let timer: ReturnType<typeof setTimeout> | null = null;

  function commonPrefixLen(a: string, b: string): number {
    const n = Math.min(a.length, b.length);
    let i = 0;
    while (i < n && a[i] === b[i]) i++;
    return i;
  }

  function cancel() {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
  }

  function step() {
    timer = null;
    if (shown === target) {
      animating = false;
      return;
    }
    const keep = commonPrefixLen(shown, target);
    if (shown.length > keep) {
      // backspace one char toward the common prefix
      shown = shown.slice(0, shown.length - 1);
    } else {
      // type the next char of the target
      shown = target.slice(0, shown.length + 1);
    }
    animating = true;
    timer = setTimeout(step, STEP_MS);
  }

  function retarget(next: string) {
    target = next;
    cancel(); // cancel any in-flight schedule; restart from current `shown`
    if (reduceMotion) {
      shown = next;
      animating = false;
      return;
    }
    if (shown === target) {
      animating = false;
      return;
    }
    animating = true;
    timer = setTimeout(step, STEP_MS);
  }

  // React to prop changes (mid-animation restarts from the current displayed string).
  $: if (text !== target) retarget(text);

  onDestroy(cancel);
</script>

<span class="tw">{shown}{#if animating}<span class="caret">▌</span>{/if}</span>

<style>
  .tw {
    display: inline;
    white-space: pre;
  }
  .caret {
    opacity: 0.7;
    animation: blink 0.9s steps(1) infinite;
  }
  @keyframes blink {
    0%,
    49% {
      opacity: 0.7;
    }
    50%,
    100% {
      opacity: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .caret {
      animation: none;
    }
  }
</style>
