import type { LLMModelView } from "@loomarr/api";

interface ModelPickerProps {
  // The BE ranks and explains fit for THIS machine (§8.1) — the UI never guesses which
  // model suits the hardware, it renders the annotation it was given.
  catalog: LLMModelView[];
  // Currently active model tag, if any.
  active?: string;
  // Detected hardware, shown so a "won't fit" verdict is explainable rather than bare.
  gpuName?: string;
  vramGiB?: number;
  onSelect: (tag: string) => void;
  onPull: (tag: string) => void;
  // The tag currently downloading. `percent` is computed by the BE and arrives on the
  // llm_pull frames from the first one — deriving it from byte counts here would show
  // nothing until bytes start flowing.
  pulling?: { tag: string; percent?: number };
  busy?: boolean;
  className?: string;
}

export type { ModelPickerProps };
