import { cn } from "@/lib";
import type { NavTabsProps } from "./nav-tabs.type";

// NavTabs — THE tab bar for this app: a pill per destination, the active one filled with a
// `signal` tint. Settings, Filler and Queue all use it, so "you are here" looks the same
// everywhere.
//
// ⚠ **Every tab is a real `<Link>`, never a button.** All three bars already keep their position
// in the URL — Settings as its own route per section, Filler and Queue as `?tab=` — so each tab
// WAS already a destination; two of them just rendered as buttons calling `navigate()`. That
// looks identical and is not the same thing: a button cannot be middle-clicked into a new tab,
// cannot be copied as a link, gets no browser affordances, and tells assistive tech it performs
// an action rather than going somewhere.
//
// ⚠ **`aria-current="page"`, not `role="tab"`.** The ARIA tab pattern describes panels swapped
// inside one document, with arrow-key roving focus — the right model for a button-driven
// tablist. These are NAVIGATION: each one changes the URL and (for Settings) mounts a different
// route. Claiming `role="tab"` here would promise arrow-key behaviour that navigation does not
// have, and axe flags a `tablist` whose tabs are links. This replaced a `CountTabs` that did
// declare `role="tab"` with hand-rolled ←/→ handling — correct for what it was, wrong for what
// these bars actually are.
//
// The active pill is the Settings treatment (maintainer's pick, 2026-08-02): `signal-tint-15`
// fill, `signal` text, `rounded-md`. It replaced an underline-only bar on Filler and Queue.
const NavTabs = ({ tabs, activeId, linkComponent: Link, label, className }: NavTabsProps) => (
  <nav aria-label={label} className={cn("flex gap-1 overflow-x-auto border-border border-b pb-2", className)}>
    {tabs.map((tab) => {
      const active = tab.id === activeId;
      return (
        <Link
          key={tab.id}
          to={tab.to}
          {...(tab.search ? { search: tab.search } : {})}
          id={`tab-${tab.id}`}
          // ⚠ `aria-current` marks the active destination for a screen reader. Without it the
          // amber fill is the ONLY signal, which is invisible to anyone not looking at colour.
          {...(active ? { "aria-current": "page" as const } : {})}
          // ⚠ There is deliberately NO `aria-controls`. It was carried over from `CountTabs`,
          // where the tabs genuinely revealed panels in the same document — but these NAVIGATE,
          // so there is no panel for a link to control, and pointing at one that may not be
          // mounted is axe's `aria-valid-attr-value` at CRITICAL impact. `aria-current` alone is
          // the right vocabulary for "this is the page you are on"; the caught violation is a
          // reminder that copying ARIA between two things that merely look alike is how a
          // component acquires attributes that promise behaviour it does not have.
          className={cn(
            "flex shrink-0 items-center gap-2 whitespace-nowrap rounded-md px-3 py-1.5 text-sm transition-colors",
            "text-muted-foreground hover:bg-accent hover:text-foreground",
            active && "bg-signal-tint-15 font-medium text-signal",
          )}
        >
          {tab.label}
          {/* ⚠ Omitted when a tab has no count, rather than rendered as "0" — see NavTab.count. */}
          {tab.count !== undefined && (
            <span
              className={cn(
                "rounded-full px-1.5 py-px font-mono text-2xs",
                // ⚠ The ACTIVE pill sits on `background`, not on a `signal` tint. Amber text on an
                // amber tint composites amber-on-amber: measured 4.11:1 against the required
                // 4.5:1 at 11px — an axe `color-contrast` failure at SERIOUS impact, and one only
                // this gate could catch. The tint was chosen for visual harmony without checking
                // it against the fill already behind it (the active tab is itself tinted).
                active ? "bg-background text-signal" : "bg-static-800 text-static-400",
              )}
            >
              {tab.count}
            </span>
          )}
        </Link>
      );
    })}
  </nav>
);

export { NavTabs };
