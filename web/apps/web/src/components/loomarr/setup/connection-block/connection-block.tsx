import { Collapsible } from "@base-ui/react/collapsible";
import { parseDocHref } from "@loomarr/core";
import { Link } from "@tanstack/react-router";
import { ArrowUpRight, Check, ChevronDown, X } from "lucide-react";
import { cn } from "@/lib";
import type { ConnectionBlockProps } from "./connection-block.type";

// ConnectionBlock — one service (Media server, Tunarr, …) as a self-diagnosing, collapsible
// block (config-design §5/§6): a header with a live status dot, the connection's fields in a
// slide-open body, and its Test verdict INLINE below them. Diagnosis lives on the thing that
// fixes it — no separate checklist to scroll between.
//
// This is the wizard's connection block (was hand-rolled in wizard/checklist-step) lifted into
// a shared primitive so the wizard and Settings/Connections are the same shell by construction,
// not two lookalikes. It shares CollapsibleSection's `.reveal` mechanics (grid 0fr→1fr,
// height-agnostic, reduced-motion frozen; styles.css) but has its own compact header — a small
// status dot on the left, not an icon — so it reads as a status row, not a heading.
//
// Controlled open: the caller drives expansion so broken blocks open and healthy ones collapse.
//
// ⚠ **A COLLAPSED BLOCK USED TO BE KEYBOARD-REACHABLE (fixed in V50d).** The hand-rolled version
// closed with `grid-template-rows: 0fr` + `overflow:hidden` — zero height but NOT `display:none` —
// so its body stayed in the accessibility tree and stayed focusable. This block's body holds a
// "Test connection" button and, when a check fails, a "Fix" link into the Help centre: a keyboard
// user tabbing through the wizard hit both while the section was visibly shut.
//
// CollapsibleSection was ported in V50c and this was left behind, which is why the two lookalikes
// disagreed for three phases. `Collapsible.Panel hiddenUntilFound` renders the closed body as
// `content-visibility: hidden` — out of the a11y tree, still reachable by the browser's
// find-in-page, and still mounted so nothing loses state.
//
// ⚠ It has no story, so **axe never sees this component** — the fix is verified by a unit test
// asserting the closed panel carries `hidden="until-found"`, not by the visual suite.
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

  return (
    <Collapsible.Root
      open={open}
      // ⚠ Controlled-only by design (the caller opens broken blocks and collapses healthy ones),
      // so `onToggle` takes no argument and Base UI's `(open, details)` is dropped deliberately
      // rather than widened into the public prop type.
      onOpenChange={() => onToggle()}
      render={<section className="overflow-hidden rounded-lg border border-border" />}
    >
      <Collapsible.Trigger className="group flex w-full cursor-pointer items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-static-800">
        {/* Status dot: green + check when the probe passes; red-ringed when it fails;
            neutral (muted, empty) when untested. It carries the resting signal so the
            header alone says "this one's fine" or "look here" — paired with the text
            summary below, because a dot is colour-only and shape-only. */}
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
        {/* The resting summary, so a COLLAPSED block still says where it stands. The dot
            alone cannot distinguish "failed" from "never tested" for anyone who does not
            read colour, and the full hint lives in the body — this is the one-line version.
            `ml-auto` here (and not on the chevron) right-aligns it as the mock does. */}
        <span
          className={cn(
            "ml-auto truncate text-xs",
            verdict?.ok && !testing && "text-lock",
            failing && !testing && "text-onair-300",
            (testing || verdict === undefined) && "text-static-400",
          )}
        >
          {summary}
        </span>
        <ChevronDown
          className="size-4 shrink-0 text-muted-foreground transition-transform group-data-[panel-open]:rotate-180"
          aria-hidden
        />
      </Collapsible.Trigger>

      {/* The reveal: grid 0fr→1fr so the body slides open with no fixed height (styles.css).
          ⚠ `.reveal` matches BOTH `[data-open="true"]` (a React boolean) and `[data-open=""]`
          (Base UI's valueless attribute); this now emits the latter. */}
      <Collapsible.Panel hiddenUntilFound className="reveal">
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
            {/* Below the fields AND below the Test row — a statement about the service, read
                after its settings rather than competing with them. */}
            {footer}
          </div>
        </div>
      </Collapsible.Panel>
    </Collapsible.Root>
  );
};

export { ConnectionBlock };
