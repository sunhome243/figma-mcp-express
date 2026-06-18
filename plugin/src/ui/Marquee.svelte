<script lang="ts">
  import { onMount, tick } from "svelte";

  export let text = "";
  export let cls = "";
  export let title = "";

  let wrap: HTMLElement;
  let clip: HTMLSpanElement | null = null;
  let scrolling = false;
  let dist = 0;
  let dur = 0;

  const GAP = 52;   // px between the two text copies
  const SPEED = 55; // px/s — longer text scrolls at the same rate
  const DELAY = 1.5; // s pause before first scroll

  async function measure() {
    if (!wrap) return;
    if (!clip) {
      // clip is only mounted when !scrolling; reset first so it remounts
      scrolling = false;
      dist = 0;
      dur = 0;
      await tick();
    }
    if (!clip) return;
    const overflows = clip.scrollWidth > wrap.clientWidth;
    if (overflows && !scrolling) {
      dist = clip.scrollWidth + GAP;
      dur = Math.max(3, dist / SPEED);
      scrolling = true;
    } else if (!overflows && scrolling) {
      scrolling = false;
      dist = 0;
      dur = 0;
    }
  }

  onMount(() => {
    measure();
    const ro = new ResizeObserver(() => measure());
    ro.observe(wrap);
    return () => ro.disconnect();
  });

  let _prev = "";
  $: if (text !== _prev) {
    _prev = text;
    if (scrolling) {
      // reset to clip state so we can re-measure the new text
      scrolling = false;
      dist = 0;
      dur = 0;
      tick().then(measure);
    }
  }
</script>

<div class="mq {cls}" bind:this={wrap} {title}>
  {#if scrolling}
    <div
      class="mq-track"
      style="gap:{GAP}px; --dur:{dur}s; --delay:{DELAY}s; --dist:{dist}px"
    >
      <span>{text}</span>
      <span aria-hidden="true">{text}</span>
    </div>
  {:else}
    <span class="mq-clip" bind:this={clip}>{text}</span>
  {/if}
</div>

<style>
  .mq {
    overflow: hidden;
    min-width: 0;
    width: 100%;
    /* inherits font-size / color from parent */
  }

  .mq-clip {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mq-track {
    display: inline-flex;
    white-space: nowrap;
    will-change: transform;
    animation: mq-run var(--dur) linear var(--delay) infinite;
  }

  @keyframes mq-run {
    from { transform: translateX(0); }
    to   { transform: translateX(calc(-1 * var(--dist))); }
  }

  @media (prefers-reduced-motion: reduce) {
    .mq-track { animation: none; }
  }
</style>
