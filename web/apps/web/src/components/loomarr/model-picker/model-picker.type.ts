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
  // The tag currently downloading, with progress from the llm_pull SSE frames.
  pulling?: { tag: string; completed?: number; total?: number };
  busy?: boolean;
  className?: string;
}

export type { ModelPickerProps };
