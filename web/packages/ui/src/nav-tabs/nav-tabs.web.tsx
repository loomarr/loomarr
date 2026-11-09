import { Text } from "@loomarr/design-system";
import { useLayoutEffect, useRef, useState } from "react";

import type { NavTab, NavTabsProps } from "./nav-tabs.type";

// The slice of the DOM the scroll-into-view effect touches. This package has no DOM lib (it targets
// React Native too), so the shape is spelled out here.
type Box = { left: number; width: number };
type Bar = {
  clientWidth: number;
  scrollLeft: number;
  getBoundingClientRect(): Box;
  querySelector(selector: string): { getBoundingClientRect(): Box } | null;
};
type Observer = { observe(target: unknown): void; disconnect(): void };
type ObserverConstructor = new (callback: () => void) => Observer;

const suggestTint = "color-mix(in srgb, var(--accentSuggest) 15%, transparent)";

// NavTabs — THE tab bar for this app: a pill per destination, the active one on the elevated
// fill. Settings, Filler, Queue and channel detail all use it, so "you are here" looks the same
// everywhere.
//
// ⚠ **Every tab is a real `<Link>`, never a button.** Each bar keeps its position in the URL, so
// each tab is a destination: a link can be middle-clicked into a new tab, copied, and tells
// assistive tech it goes somewhere rather than performing an action.
//
// ⚠ **`aria-current="page"`, not `role="tab"`.** The ARIA tab pattern describes panels swapped
// inside one document with arrow-key roving focus. These are NAVIGATION: claiming `role="tab"`
// would promise arrow-key behaviour navigation does not have, and axe flags a `tablist` whose tabs
// are links. There is deliberately no `aria-controls` either: there is no panel for a link to
// control, and pointing at one that may not be mounted is axe's `aria-valid-attr-value`.
//
// The active pill is neutral: the elevated fill and primary text. Amber means focus or the primary
// action, so "you are here" matches the phone bottom bar (#1909 Q1).
//
// ⚠ **Two looks, one component.** `variant="underline"` is the v2 mock's Queue tab bar: transparent
// tabs, a 2px underline in the primary colour on the active one, over a 1px rule under the bar.
const NavTabs = ({ activeId, label, linkComponent: Link, tabs, variant = "pill" }: NavTabsProps) => {
  const underline = variant === "underline";
  const [hovered, setHovered] = useState<string>();
  // One line at every width, scrolling sideways when the tabs don't fit (#1785). The active tab is
  // scrolled into view, sideways only: the page itself never moves. It is scrolled again when the
  // tab or the bar changes size: the web font arriving widens every tab after the first paint.
  const bar = useRef<Bar>(null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: activeId IS the dependency; the effect reads the DOM it marks
  useLayoutEffect(() => {
    const nav = bar.current;
    const current = nav?.querySelector("[aria-current=page]");
    if (!nav || !current) return;
    const reveal = () => {
      const box = current.getBoundingClientRect();
      const start = box.left - nav.getBoundingClientRect().left + nav.scrollLeft;
      const end = start + box.width;
      if (start < nav.scrollLeft) nav.scrollLeft = start;
      else if (end > nav.scrollLeft + nav.clientWidth) nav.scrollLeft = end - nav.clientWidth;
    };
    reveal();
    const Resize = (globalThis as { ResizeObserver?: ObserverConstructor }).ResizeObserver;
    if (!Resize) return;
    const observer = new Resize(reveal);
    observer.observe(current);
    observer.observe(nav);
    return () => observer.disconnect();
  }, [activeId]);

  const tabStyle = (tab: NavTab, active: boolean) => {
    const lit = active || hovered === tab.id;
    const common = {
      alignItems: "center",
      color: lit ? "var(--contentPrimary)" : "var(--contentSecondary)",
      display: "flex",
      flexShrink: 0,
      fontWeight: active ? 500 : 400,
      gap: 8,
      textDecoration: "none",
      transition: "color 150ms, background-color 150ms",
      whiteSpace: "nowrap",
    };
    return underline
      ? {
          ...common,
          // The -1px margin lets the 2px underline sit over the bar's 1px rule.
          backgroundColor: "transparent",
          borderBottom: `2px solid ${active ? "var(--contentPrimary)" : "transparent"}`,
          fontSize: 13.5,
          marginBottom: -1,
          padding: "9px 13px",
        }
      : {
          ...common,
          // The #1659 web mock's pill: 13px at the browser's normal line height, and hover
          // changes the colour only.
          backgroundColor: active ? "var(--surfaceElevated)" : "transparent",
          borderRadius: 6,
          fontSize: 13,
          lineHeight: "normal",
          padding: "6px 12px",
        };
  };

  // ⚠ The ACTIVE pill's count sits on the canvas, so it reads as a chip against the elevated pill.
  // A count tinted to match the fill behind it once failed axe `color-contrast`. Waiting work
  // outranks "you are here" (#1659 web mock): Needs you keeps its suggest count while selected.
  const countStyle = (tab: NavTab, active: boolean) => {
    const attention = tab.attention && (tab.count ?? 0) > 0;
    return {
      backgroundColor: attention
        ? suggestTint
        : active && !underline
          ? "var(--surfaceCanvas)"
          : "var(--surfaceElevated)",
      borderRadius: 999,
      color: attention ? "var(--accentSuggest)" : active && !underline ? "var(--contentPrimary)" : undefined,
      padding: underline ? "1px 7px" : "1px 6px",
    };
  };

  return (
    <nav
      aria-label={label}
      ref={bar as never}
      style={{
        borderBottom: "1px solid var(--borderDecorative)",
        display: "flex",
        flexShrink: 0,
        gap: 4,
        overflowX: "auto",
        paddingBottom: underline ? 0 : 8,
        // The underline tab's -1px margin overflows the bar by a pixel, which `overflow-x: auto`
        // turns into a vertical scrollbar; the bar scrolls sideways without drawing one (#2738).
        scrollbarWidth: "none",
      }}
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
            // fill is the ONLY signal, which is invisible to anyone not looking at it.
            {...(active ? { "aria-current": "page" as const } : {})}
            onMouseEnter={() => setHovered(tab.id)}
            onMouseLeave={() => setHovered(undefined)}
            style={tabStyle(tab, active)}
          >
            {tab.label}
            {/* ⚠ Omitted when a tab has no count, rather than rendered as "0" — see NavTab.count. */}
            {tab.count !== undefined && (
              <Text as="span" style={countStyle(tab, active)} textRole="captionMono">
                {tab.count}
              </Text>
            )}
          </Link>
        );
      })}
    </nav>
  );
};

export { NavTabs };
