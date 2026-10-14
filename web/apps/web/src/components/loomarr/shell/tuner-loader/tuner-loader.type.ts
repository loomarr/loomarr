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
  className?: string;
}

export type { TunerLoaderProps };
