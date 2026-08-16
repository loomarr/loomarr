import { parseDocHref } from "@loomarr/core/anchor";
import { Link } from "@tanstack/react-router";
import { ArrowUpRight, Circle, CircleCheck, CircleX, Loader2 } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import type { ChecklistItemProps, CheckStatus } from "./checklist-item.type";

// ChecklistItem — the wizard / Settings-troubleshooting check row (§3, §6). A pass
// "locks in" green (§2.4); a fail never blames — plain cause + the exact fix + a
// deep link into the embedded docs (§13). Backed by GET /v1/setup/status.
const ICON: Record<CheckStatus, { node: ReactNode; label: string }> = {
  pending: { node: <Circle className="size-4 text-static-500" aria-hidden />, label: "Pending" },
  running: { node: <Loader2 className="size-4 animate-spin text-tune" aria-hidden />, label: "Checking" },
  pass: { node: <CircleCheck className="size-4 text-lock" aria-hidden />, label: "Passed" },
  fail: { node: <CircleX className="size-4 text-onair-300" aria-hidden />, label: "Failed" },
};

const ChecklistItem = ({ name, status, hint, docHref, className }: ChecklistItemProps) => (
  <div
    className={cn(
      "flex items-start gap-3 rounded-md border border-border bg-card px-3 py-2.5 transition-colors",
      status === "pass" && "border-lock-tint-30",
      status === "fail" && "border-onair-tint-30",
      className,
    )}
  >
    <span className="mt-0.5" role="img" aria-label={ICON[status].label}>
      {ICON[status].node}
    </span>
    <div className="min-w-0 flex-1">
      <p className="font-medium text-sm">{name}</p>
      {status === "fail" && hint && <p className="mt-0.5 text-muted-foreground text-sm">{hint}</p>}
    </div>
    {status === "fail" && docHref && (
      // Route into the Help center — the docHref ("troubleshooting#tunarr") is parsed
      // into { page, section }, not used as a raw href (which resolves relative to the
      // current path and 404s). This is what makes §13's "every red check deep-links to
      // its section" actually navigate.
      <Link
        to="/help"
        search={parseDocHref(docHref)}
        className="inline-flex shrink-0 items-center gap-0.5 text-tune text-xs hover:underline"
      >
        Fix <ArrowUpRight className="size-3" aria-hidden />
      </Link>
    )}
  </div>
);

export { ChecklistItem };
