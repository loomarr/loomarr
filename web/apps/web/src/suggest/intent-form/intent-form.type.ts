import type { Intent } from "@loomarr/api/models/intent";

interface IntentFormProps {
  // Prefills the complete typed intent. A legacy free-text Guide link is represented as
  // an Intent with only `description`; a preset also carries its authored constraints.
  initialIntent?: Intent;
  onSubmit: (intent: Intent) => void;
  submitting?: boolean;
}

export type { IntentFormProps };
