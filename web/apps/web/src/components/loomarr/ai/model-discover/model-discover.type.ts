import type { DiscoverModelView } from "@loomarr/api";

interface ModelDiscoverProps {
  // The compatible downloadable models, sized + fit-ranked for THIS machine by the BE
  // (§8.1). Undefined = still loading; empty = none compatible / source unreachable.
  results?: DiscoverModelView[];
  // Discovery is in flight.
  loading?: boolean;
  // The source couldn't be reached — the UI degrades to a "browse on huggingface.co"
  // link rather than pretending nothing is compatible (§8.1 best-effort).
  sourceError?: boolean;
  // The pullRef currently downloading, so its Download button shows progress and the
  // rest stay enabled. `percent` arrives on the BE-computed llm_pull frames.
  pulling?: { tag: string; percent?: number };
  // Pull a candidate by its (quant-targeted) pullRef.
  onPull: (ref: string) => void;
  className?: string;
}

export type { ModelDiscoverProps };
