import { cn } from "@/lib";
import type { CountTabsProps } from "./count-tabs.type";

// CountTabs — an underlined tab bar where each tab carries a count pill (the v2 mock's Queue
// tabs: `Needs approval 3 · In flight 12 · History 14`).
//
// Deliberately NOT `ChannelNav`, which is otherwise close enough to be tempting. That one is a
// filled-pill active state with no counts; this one is an underline with a count pill, which is
// what the mock draws. Forcing one component to be both would mean a variant flag whose two
// branches share only the word "tabs" — and the count is not decoration here, it is the reason
// the bar exists (an admin's first question is "how many need me?").
//
// Proper `tablist`/`tab` semantics with `aria-controls`, so a screen reader announces "tab 2 of
// 3" and the selected state, rather than hearing three unrelated buttons.
const CountTabs = ({ tabs, activeId, onSelect, label, className }: CountTabsProps) => (
  <div
    role="tablist"
    aria-label={label}
    className={cn("flex gap-1 overflow-x-auto border-border border-b", className)}
  >
    {tabs.map((t) => {
      const active = t.id === activeId;
      return (
        <button
          key={t.id}
          type="button"
          role="tab"
          id={`tab-${t.id}`}
          aria-selected={active}
          // ONLY on the active tab. Just one panel is mounted at a time, so pointing an
          // inactive tab at `panel-<its-id>` references an element that does not exist —
          // axe's `aria-valid-attr-value`, at serious impact. (Caught by CI's a11y sweep after
          // a local `--update-snapshots` run reported green: updating snapshots is not
          // verification.)
          {...(active ? { "aria-controls": `panel-${t.id}` } : {})}
          // Only the active tab is in the tab sequence; ←/→ move between them, which is the
          // standard tablist pattern (and the same aria-driven model the ⌘K palette uses).
          tabIndex={active ? 0 : -1}
          onKeyDown={(e) => {
            if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
            e.preventDefault();
            const i = tabs.findIndex((x) => x.id === activeId);
            const next = e.key === "ArrowRight" ? (i + 1) % tabs.length : (i - 1 + tabs.length) % tabs.length;
            const target = tabs[next];
            if (target) {
              onSelect(target.id);
              document.getElementById(`tab-${target.id}`)?.focus();
            }
          }}
          onClick={() => onSelect(t.id)}
          data-status={active ? "active" : undefined}
          className={cn(
            "-mb-px flex shrink-0 cursor-pointer items-center gap-2 whitespace-nowrap border-b-2 px-3 py-2 text-sm transition-colors",
            "border-transparent text-muted-foreground hover:text-foreground",
            "data-[status=active]:border-signal data-[status=active]:font-medium data-[status=active]:text-signal",
          )}
        >
          {t.label}
          <span
            className={cn(
              "rounded-full px-1.5 py-px font-mono text-2xs",
              active ? "bg-signal-tint-15 text-signal" : "bg-static-800 text-static-400",
            )}
          >
            {t.count}
          </span>
        </button>
      );
    })}
  </div>
);

export { CountTabs };
