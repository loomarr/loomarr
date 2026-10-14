import type { Intent } from "@loomarr/api/models/intent";

interface IntentFormProps {
  // Prefills the description — the wizard's guided first channel hands off a template
  // this way (§13), and the ⌘K palette will too.
  initialDescription?: string;
  // A failed Journey can return the complete server-owned intent, including its
  // constraints, for an authorized edit.
  initialIntent?: Intent;
  onSubmit: (intent: Intent) => void;
  submitting?: boolean;
}

export type { IntentFormProps };
