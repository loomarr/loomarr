import type { CreateLocalUserInputBody } from "@loomarr/api/models/createLocalUserInputBody";

interface CreateLocalDialogProps {
  creating?: boolean;
  defaultOpen?: boolean;
  error?: unknown;
  portalContainer?: HTMLElement | ShadowRoot | null;
  onCreate: (input: CreateLocalUserInputBody) => Promise<unknown> | unknown;
}

export type { CreateLocalDialogProps };
