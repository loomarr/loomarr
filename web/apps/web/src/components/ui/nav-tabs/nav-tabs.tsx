import { useLayoutEffect, useRef } from "react";
import { cn } from "@/lib/utils";
import type { NavTabsProps } from "./nav-tabs.type";

// NavTabs — THE tab bar for this app: a pill per destination, the active one on the elevated
// fill. Settings, Filler and Queue all use it, so "you are here" looks the same everywhere.
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
// The active pill is neutral: `static-800` (elevated) fill, foreground text at weight 500,
// `rounded-md`. Amber means focus or the primary action, so "you are here" matches the phone
// bottom bar (#1909 Q1, https://github.com/loomarr/loomarr/pull/1909). That decision overturns
// the amber `signal-tint-15` pill picked for Settings on 2026-08-02.
//
// ⚠ **Two looks, one component.** `variant="underline"` is the v2 mock's Queue tab bar (Requests):
// transparent tabs, a 2px bottom border in foreground on the active one, and a 1px rule under the
// bar that the active underline overlaps. The pill stays the default because Settings, Filler and
// channel detail are drawn with pills in their own mocks — the variant exists so a screen follows
// ITS mock rather than every bar being forced into one style.
const NavTabs = ({
  tabs,
  activeId,
  linkComponent: Link,
  label,
  variant = "pill",
  className,
}: NavTabsProps) => {
  const underline = variant === "underline";
  // One line at every width, scrolling sideways when the tabs don't fit (#1785). At phone width it
  // used to wrap, so a channel's Danger zone or Requests' Done sat on a second row. The active tab
  // is scrolled into view, sideways only: the page itself never moves. It is scrolled again when the
  // tab or the bar changes size: the web font arriving widens every tab after the first paint.
  const bar = useRef<HTMLElement>(null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: activeId IS the dependency; the effect reads the DOM it marks
  useLayoutEffect(() => {
    const nav = bar.current;
    const current = nav?.querySelector<HTMLElement>("[aria-current=page]");
    if (!nav || !current) return;
    const reveal = () => {
      const box = current.getBoundingClientRect();
      const start = box.left - nav.getBoundingClientRect().left + nav.scrollLeft;
      const end = start + box.width;
      if (start < nav.scrollLeft) nav.scrollLeft = start;
      else if (end > nav.scrollLeft + nav.clientWidth) nav.scrollLeft = end - nav.clientWidth;
    };
    reveal();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(reveal);
    observer.observe(current);
    observer.observe(nav);
    return () => observer.disconnect();
  }, [activeId]);
  return (
    <nav
      ref={bar}
      aria-label={label}
      className={cn(
        "flex shrink-0 gap-1 overflow-x-auto border-border border-b",
        !underline && "pb-2",
        className,
      )}
    >
      {tabs.map((tab) => {
        const active = tab.id === activeId;
        return (
          <Link
            key={tab.id}
            to={tab.to}
            {...(tab.search ? { search: tab.search } : {})}
            // The injected router link otherwise prefix-matches a parent destination such as
            // `/filler` while the operator is on `/filler/incoming`, creating two current pages.
            // Search params refine a destination; they never make its navigation entry inactive.
            activeOptions={{ exact: true, includeSearch: false }}
            id={`tab-${tab.id}`}
            // ⚠ `aria-current` marks the active destination for a screen reader. Without it the
            // elevated fill is the ONLY signal, which is invisible to anyone not looking at it.
            {...(active ? { "aria-current": "page" as const } : {})}
            // ⚠ There is deliberately NO `aria-controls`. It was carried over from `CountTabs`,
            // where the tabs genuinely revealed panels in the same document — but these NAVIGATE,
            // so there is no panel for a link to control, and pointing at one that may not be
            // mounted is axe's `aria-valid-attr-value` at CRITICAL impact. `aria-current` alone is
            // the right vocabulary for "this is the page you are on"; the caught violation is a
            // reminder that copying ARIA between two things that merely look alike is how a
            // component acquires attributes that promise behaviour it does not have.
            className={cn(
              "flex shrink-0 items-center gap-2 whitespace-nowrap transition-colors",
              underline
                ? cn(
                    // `-mb-px` lets the 2px underline sit over the bar's 1px rule.
                    "-mb-px border-b-2 bg-transparent px-[13px] py-[9px] text-[13.5px] hover:text-foreground",
                    active
                      ? "border-foreground font-medium text-foreground"
                      : "border-transparent text-muted-foreground",
                  )
                : cn(
                    // The #1659 web mock's pill: 13 px at the browser's normal line height (29 px
                    // tall, not the body's 1.5), and hover changes the colour only.
                    "rounded-md px-3 py-1.5 text-muted-foreground text-sm leading-[normal] hover:text-foreground",
                    active && "bg-static-800 font-medium text-foreground",
                  ),
            )}
          >
            {tab.label}
            {/* ⚠ Omitted when a tab has no count, rather than rendered as "0" — see NavTab.count. */}
            {tab.count !== undefined && (
              <span
                className={cn(
                  "rounded-full px-1.5 py-px font-mono text-2xs",
                  // ⚠ The ACTIVE pill's count sits on `background`, so it reads as a chip against
                  // the elevated pill. A count tinted to match the fill behind it once failed axe
                  // `color-contrast` (4.11:1 at 11px, SERIOUS); check a new tint against the pill.
                  underline
                    ? tab.attention && tab.count > 0
                      ? "bg-suggest-tint-15 px-[7px] text-suggest-300"
                      : "bg-static-800 px-[7px] text-static-400"
                    : // Waiting work outranks "you are here" (#1659 web mock): Needs you keeps its pink
                      // count while selected, so the number still reads as work to do.
                      tab.attention && tab.count > 0
                      ? "bg-suggest-tint-15 text-suggest-300"
                      : active
                        ? "bg-background text-foreground"
                        : "bg-static-800 text-static-400",
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
};

export { NavTabs };
