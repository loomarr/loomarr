import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

// PanelRow — the shared list-row for dashboard panels (Playout, Services, Activity, …).
//
// It exists to own ONE contract in one place: a row whose content WRAPS to a second line when the
// viewport is narrow instead of truncating or being clipped. Panels used to hand-roll the flex
// incantation per row, and a subtly-wrong version (a fixed-width right cluster with no `min-w-0` on
// the row) overflowed and was CLIPPED by the card's overflow-hidden — visibly in Firefox, which
// computes flex `min-width:auto` differently from Chromium. Centralising it means no panel re-derives
// the incantation and none can reintroduce that bug.
//
// Compose it: `<PanelRow><PanelRow.Main>…</PanelRow.Main><PanelRow.Meta>…</PanelRow.Meta></PanelRow>`.
// A row with no right-hand cluster (Activity) just puts its content directly in PanelRow.

interface PanelRowProps {
  children: ReactNode;
  className?: string;
}

// The row container: wraps, aligns, and separates. `min-w-0` is load-bearing (see the file header) —
// without it a flex child refuses to shrink below its content and the row overflows.
const PanelRow = ({ children, className }: PanelRowProps) => (
  <li
    className={cn(
      "flex min-w-0 flex-wrap items-center justify-between gap-x-4 gap-y-2 border-border border-b px-4 py-3 last:border-b-0",
      className,
    )}
  >
    {children}
  </li>
);

// Main — the primary/identity region. Grows to fill and truncates its own overflow; `basis-64` is
// what makes the Meta cluster wrap to a second line once the row is narrower than ~identity + cluster,
// rather than either side being cut off.
const Main = ({ children, className }: PanelRowProps) => (
  <div className={cn("min-w-0 flex-1 basis-64", className)}>{children}</div>
);

// Meta — the trailing cluster (numbers, badges, actions). Stays intact as one unit and drops to its
// own line when the row wraps, so it is never clipped.
const Meta = ({ children, className }: PanelRowProps) => (
  <div className={cn("flex shrink-0 items-center gap-3", className)}>{children}</div>
);

PanelRow.Main = Main;
PanelRow.Meta = Meta;

export { PanelRow };
