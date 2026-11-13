interface TunerLoaderProps {
  /** Status line under the bars. Defaults to "TUNING IN" (the Watch player's warm-up). */
  label?: string;
  /** The channel being tuned, named between the bars and the status line ("CH n" + name). */
  channel?: { number: number | string; name: string };
  /**
   * True when a picture from the previous channel is held under the loader (a channel switch): the
   * wash drains it into the snow. False on a cold start, where there is nothing to drain and the
   * wash simply rests at its drained look.
   */
  heldFrame?: boolean;
  /**
   * The programme's fanart, for a long wait (1.5 s and up, §9.4): shown dimmed behind the snow, contained
   * at 16:9 (letterboxed, never stretched), so the wait shows what is on.
   */
  backdrop?: string;
  className?: string;
}

export type { TunerLoaderProps };
