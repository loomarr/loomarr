import type { ImportCandidate } from "@loomarr/api";

interface ImportDialogProps {
  available: boolean;
  candidates?: ImportCandidate[];
  candidateError?: unknown;
  mutationError?: unknown;
  importing?: boolean;
  syncing?: boolean;
  defaultOpen?: boolean;
  portalContainer?: HTMLElement | ShadowRoot | null;
  onRetry?: () => void;
  onImport: (ids: string[]) => Promise<unknown> | unknown;
  onSync: () => Promise<unknown> | unknown;
}

export type { ImportDialogProps };
