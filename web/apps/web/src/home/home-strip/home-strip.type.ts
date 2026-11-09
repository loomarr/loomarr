type HomeStripState = "loading" | "error" | "empty" | "ok";

interface HomeStripProps {
  state: HomeStripState;
  isAdmin: boolean;
  // "Couldn't load your channels." → Try again (initial error, #1822 evidence).
  onRetry: () => void;
}

export type { HomeStripProps, HomeStripState };
