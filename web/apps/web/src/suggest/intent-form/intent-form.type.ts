import type { Intent } from "@loomarr/api";

interface IntentFormProps {
  // Prefills the description — the wizard's guided first channel hands off a template
  // this way (§13), and the ⌘K palette will too.
  initialDescription?: string;
  onSubmit: (intent: Intent) => void;
  submitting?: boolean;
}

export type { IntentFormProps };
