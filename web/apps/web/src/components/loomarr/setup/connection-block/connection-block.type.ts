import type { ReactNode } from "react";

// The live verdict for a connection: whether its probe passed, and the human hint the BE
// returned (an error to fix, or a success line). `undefined` = not tested / unknown yet.
interface ConnectionVerdict {
  ok: boolean;
  hint?: string;
}

interface ConnectionBlockProps {
  // The connection's name, shown in the header (e.g. "Media server", "Tunarr").
  title: string;
  // Marks the block "optional" in the header — connections the operator can wire up later
  // (Requester, TMDB) rather than the two that must pass. Purely a label.
  optional?: boolean;
  // The standing/last verdict. Drives the header status dot (green when ok), the header's
  // one-line summary, and the inline verdict line under the fields. `undefined` renders a
  // neutral, untested dot and no line.
  verdict?: ConnectionVerdict;
  // This block's probe is in flight — the header summary reads "testing…" instead of the
  // standing verdict. Purely presentational; the caller owns which block is being tested.
  testing?: boolean;
  // The BE's docHref for this check (e.g. "troubleshooting#tunarr"). When the verdict is
  // failing, a "Fix →" link routes into the Help center via parseDocHref — NOT a raw href
  // (which resolves relative to the current path and 404s; see ChecklistItem).
  docHref?: string;
  // Controlled open state — the caller drives which blocks are expanded (broken open, healthy
  // collapsed), so open state can react to the live checklist.
  open: boolean;
  onToggle: () => void;
  // The fields (a SettingsFields group). Rendered in the slide-open body.
  children: ReactNode;
  // The action row below the fields — the "Test connection" button. Rendered beside the
  // verdict line so diagnosis and its re-test sit together (the wizard's arrangement).
  action?: ReactNode;
}

export type { ConnectionBlockProps, ConnectionVerdict };
