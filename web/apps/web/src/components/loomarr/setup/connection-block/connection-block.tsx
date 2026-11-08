import { parseDocHref } from "@loomarr/core/anchor";
import { Disclosure, type StatusTone } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { ArrowUpRight, X } from "lucide-react";
import { cn } from "@/lib/utils";
import type { ConnectionBlockProps } from "./connection-block.type";

// ConnectionBlock — one service (Media server, Tunarr, …) as a self-diagnosing, collapsible
// block (config-design §5/§6): a header with a live status dot, the connection's fields in the
// body, and its Test verdict INLINE below them. Diagnosis lives on the thing that fixes it — no
// separate checklist to scroll between.
//
// The wizard and Settings/Connections share this shell, and it is the DS Disclosure: the status
// dot is the Disclosure's `status`, and the one-line summary is its description, the words a
// colour-only dot needs beside it.
//
// Controlled open: the caller drives expansion so broken blocks open and healthy ones collapse.
//
// ⚠ A collapsed body must stay out of the tab order. Its "Test connection" button and "Fix" link
// were once reachable while the block was visibly shut (V50d). The Disclosure keeps a collapsed
// body mounted under `hidden="until-found"`: out of the accessibility tree, reachable by the
// browser's find-in-page, and no half-filled form loses its state.
const ConnectionBlock = ({
  title,
  optional = false,
  verdict,
  testing = false,
  docHref,
  open,
  onToggle,
  children,
  action,
  footer,
}: ConnectionBlockProps) => {
  const failing = verdict !== undefined && !verdict.ok;
  // The header's one-line summary (the v2 mock's `bk.summary`), DERIVED here rather than
  // sent by the backend: it is display text assembled from state the client already holds,
  // and a `summary` field on `SetupCheck` would put copy in the API for no new information.
  // The mock's own vocabulary — pass/fail/running — with "not tested yet" standing in for
  // its `c.target`, which we do not carry.
  const summary = testing
    ? "testing…"
    : verdict === undefined
      ? "not tested yet"
      : verdict.ok
        ? "OK"
        : "needs attention";
  const status: StatusTone = testing
    ? "progress"
    : verdict === undefined
      ? "off"
      : verdict.ok
        ? "ok"
        : "error";

  return (
    <Disclosure
      description={optional ? `optional · ${summary}` : summary}
      expanded={open}
      label={title}
      // ⚠ Controlled-only by design (the caller opens broken blocks and collapses healthy ones),
      // so `onToggle` takes no argument.
      onExpandedChange={() => onToggle()}
      status={status}
    >
      <div className="flex flex-col gap-4">
        {children}
        {(action || verdict) && (
          <div className="flex flex-wrap items-center gap-3">
            {action}
            {verdict && (
              <p
                role="status"
                className={cn(
                  "flex items-center gap-1.5 text-sm",
                  verdict.ok ? "text-lock" : "text-onair-300",
                )}
              >
                {failing && <X className="size-3.5 shrink-0" aria-hidden />}
                <span className="min-w-0">
                  {verdict.hint ?? (verdict.ok ? "Connection OK" : "Not connected yet")}
                </span>
                {failing && docHref && (
                  // Route into the Help center via parseDocHref — a raw href would
                  // resolve relative to /settings and 404 (mirrors ChecklistItem).
                  <Link
                    to="/help"
                    search={parseDocHref(docHref)}
                    className="hover:text-onair-200 inline-flex shrink-0 items-center gap-0.5 underline underline-offset-2"
                  >
                    Fix <ArrowUpRight className="size-3" aria-hidden />
                  </Link>
                )}
              </p>
            )}
          </div>
        )}
        {/* Below the fields AND below the Test row — a statement about the service, read
            after its settings rather than competing with them. */}
        {footer}
      </div>
    </Disclosure>
  );
};

export { ConnectionBlock };
