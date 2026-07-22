import { cn } from "@/lib";
import type { ChannelNavProps } from "./channel-nav.type";

// ChannelNav — the channel detail page's section bar: a HORIZONTAL row of tabs under the page
// header (Channel info · Refine · Lineup · …). Clicking a tab expands + scrolls to that section;
// the tab for the section currently in view is highlighted (scroll-spy) in brand amber (signal)
// — the "you are here" cue, the same active-item idiom as the settings nav.
//
// Horizontal (not a vertical rail beside the app's sidebar — two parallel menus read as
// cluttered). It's a sticky bar so it stays put as the content scrolls; on narrow screens it
// scrolls sideways rather than wrapping. Presentational + controlled — the page owns activeId
// (from useSectionSpy) and handles onSelect.
const ChannelNav = ({ sections, activeId, onSelect, className }: ChannelNavProps) => (
  <nav
    aria-label="Channel sections"
    className={cn(
      "flex gap-1 overflow-x-auto border-border border-b bg-background/80 px-6 py-2 backdrop-blur",
      className,
    )}
  >
    {sections.map((s) => {
      const active = s.id === activeId;
      return (
        <button
          key={s.id}
          type="button"
          onClick={() => onSelect(s.id)}
          aria-current={active ? "true" : undefined}
          data-status={active ? "active" : undefined}
          className={cn(
            "shrink-0 cursor-pointer whitespace-nowrap rounded-md px-3 py-1.5 text-sm transition-colors",
            "text-muted-foreground hover:bg-accent hover:text-foreground",
            "data-[status=active]:bg-signal-tint-15 data-[status=active]:font-medium data-[status=active]:text-signal",
          )}
        >
          {s.label}
        </button>
      );
    })}
  </nav>
);

export { ChannelNav };
