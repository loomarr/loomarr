import type { ReactNode } from "react";

interface PreviewGroupProps {
  children: ReactNode;
  /** Invalidate an anchor when the underlying content window changes. */
  resetKey?: unknown;
}

interface PreviewAnchorProps {
  children: ReactNode;
  content?: ReactNode;
}

export type { PreviewAnchorProps, PreviewGroupProps };
