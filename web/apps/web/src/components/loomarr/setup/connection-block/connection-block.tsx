import { parseDocHref } from "@loomarr/core";
import { Link } from "@tanstack/react-router";
import { ArrowUpRight, Check, ChevronDown, X } from "lucide-react";
import { useId } from "react";
import { cn } from "@/lib";
import type { ConnectionBlockProps } from "./connection-block.type";

// ConnectionBlock — one service (Media server, Tunarr, …) as a self-diagnosing, collapsible
// block (config-design §5/§6): a header with a live status dot, the connection's fields in a
// slide-open body, and its Test verdict INLINE below them. Diagnosis lives on the thing that
// fixes it — no separate checklist to scroll between.
//
// This is the wizard's connection block (was hand-rolled in wizard/checklist-step) lifted into
// a shared primitive so the wizard and Settings/Connections are the same shell by construction,
// not two lookalikes. It borrows CollapsibleSection's `.reveal` mechanics (grid 0fr→1fr,
// height-agnostic, reduced-motion frozen; styles.css) but has its own compact header — a small
// status dot on the left, not an icon — so it reads as a status row, not a heading.
//
// Controlled open: the caller drives expansion so broken blocks open and healthy ones collapse.
const ConnectionBlock = ({
  title,
  optional = false,
  verdict,
  docHref,
  open,
  onToggle,
  children,
  action,
}: ConnectionBlockProps) => {
  const bodyId = useId();
  const failing = verdict !== undefined && !verdict.ok;

  return (
    <section className="overflow-hidden rounded-lg border border-border">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        aria-controls={bodyId}
        className="flex w-full cursor-pointer items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-static-800"
      >
        {/* Status dot: green + check when the probe passes; red-ringed when it fails;
            neutral (muted, empty) when untested. It carries the resting signal so the
            header alone says "this one's fine" or "look here". */}
        <span
          className={cn(
            "flex size-5 shrink-0 items-center justify-center rounded-full border",
            verdict?.ok && "border-lock bg-lock-tint-15 text-lock",
            failing && "border-onair-300 text-onair-300",
            verdict === undefined && "border-border text-static-400",
          )}
          aria-hidden
        >
          {verdict?.ok ? <Check className="size-3" /> : failing ? <X className="size-3" /> : null}
        </span>
        <span className="font-medium text-sm">{title}</span>
        {optional && <span className="text-static-400 text-xs">optional</span>}
        <ChevronDown
          className={cn("ml-auto size-4 text-muted-foreground transition-transform", open && "rotate-180")}
          aria-hidden
        />
      </button>

      {/* The reveal: grid 0fr→1fr so the body slides open with no fixed height (styles.css). */}
      <div id={bodyId} className="reveal" data-open={open}>
        <div className="reveal-inner">
          <div className="flex flex-col gap-4 border-border border-t px-4 py-4">
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
                        className="inline-flex shrink-0 items-center gap-0.5 underline underline-offset-2 hover:text-onair-200"
                      >
                        Fix <ArrowUpRight className="size-3" aria-hidden />
                      </Link>
                    )}
                  </p>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </section>
  );
};

export { ConnectionBlock };
