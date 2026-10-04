type HomeStripState = "loading" | "error" | "empty" | "ok";

interface HomeStripProps {
  state: HomeStripState;
  // Every channel the guide read, for the neutral count.
  count: number;
  isAdmin: boolean;
  // "Couldn't load your channels." → Try again (initial error, #1822 evidence).
  onRetry: () => void;
}

export type { HomeStripProps, HomeStripState };
