interface TvStaticProps {
  /**
   * `idle` (default): the faint sparse snow behind idle surfaces, absent under reduced motion.
   * `wash`: the tuner's channel-switch snow (#1620 B4), the same analog snow, opaque, under static-950 at 90%.
   * It is what the viewer lands on instead of a picture, so it stays as still snow under reduced
   * motion (only the flicker stops).
   */
  variant?: "idle" | "wash";
  className?: string;
}

export type { TvStaticProps };
