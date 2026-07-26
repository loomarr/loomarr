import { Search } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { Badge } from "@/components/ui";
import { cn } from "@/lib";
import type { PaletteScope, SearchCommandProps } from "./search-command.type";

// SearchCommand — the ⌘K palette (§3, §7.2) over library/TMDB/clips + channels + help.
// In-library results wear a `lock` flag so you see at a glance what's ready now vs.
// what would be an acquisition (§8). Presentational + controlled; the global ⌘K trigger
// lives in `usePaletteShortcut`.
//
// KEYBOARD + LISTBOX SEMANTICS are implemented here, hand-rolled. §12 described this as
// cmdk/shadcn `Command`, which was never true — a surface audit filed it as drift claim
// `:757`, and it was a real gap: a mouse could pick a result and a keyboard could not.
// Note CI's axe sweep passed the whole time, because missing arrow-key navigation is a
// keyboard-INTERACTION defect, not a static-markup violation. Automated a11y checks do not
// substitute for actually pressing the keys.
//
// Hand-rolled rather than adopting cmdk: cmdk is not a dependency today, and adding one
// requires a §14 change in the same PR. The pattern below is small enough not to warrant it.
//
// `aria-activedescendant` rather than roving `tabindex`: DOM focus must STAY in the input
// (the user is mid-type) while the *active option* moves. Moving focus onto each option
// would break typing.
//
// Escape is deliberately NOT handled here. The ⌘K palette already binds it at the window
// level (`useCommandShortcut`), so handling it again would close twice; the other three
// consumers (lineup editor, series scope, filler clips) each own a Cancel affordance.
const SCOPE_LABEL: Record<PaletteScope, string> = {
  channels: "Channels",
  library: "In your library",
  tmdb: "Discover (TMDB)",
  clips: "Filler clips",
  help: "Help",
};

const SCOPE_ORDER: PaletteScope[] = ["channels", "library", "tmdb", "clips", "help"];

const SearchCommand = ({
  query,
  onQueryChange,
  results,
  onSelect,
  loading = false,
  className,
}: SearchCommandProps) => {
  const listboxId = useId();
  const optionId = (index: number) => `${listboxId}-option-${index}`;

  const groups = SCOPE_ORDER.map((scope) => ({
    scope,
    items: results.filter((r) => r.scope === scope),
  })).filter((g) => g.items.length > 0);
  const empty = !loading && results.length === 0;

  // The keyboard traverses ONE flat sequence in the order the groups render, so ↓ crosses a
  // scope heading rather than stopping at it. Derived from `groups` (not `results`) so the
  // index always matches what is on screen — the groups are re-ordered by SCOPE_ORDER, and
  // indexing the raw prop would make ↓ jump around unpredictably.
  const flat = groups.flatMap((g) => g.items);

  const [active, setActive] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);

  // Reset to the first result whenever the result set changes: after typing another letter,
  // "the 4th item" is a different title, so keeping the old index would leave the highlight on
  // something the user never looked at. Keyed on the joined ids, not the array identity, which
  // is a fresh object every render. A NUL cannot occur inside an id, so the join is unambiguous.
  //
  // Adjusted DURING RENDER rather than in an effect: React's documented pattern for state
  // derived from props. An effect would paint the stale highlight for a frame first, and
  // `setActive(0)` never *reads* the key, so it is not a real effect dependency either.
  const resultKey = flat.map((r) => r.id).join("\u0000");
  const [prevKey, setPrevKey] = useState(resultKey);
  if (prevKey !== resultKey) {
    setPrevKey(resultKey);
    setActive(0);
  }

  // Keep the active option in view when it moves off-screen (the list scrolls at max-h-80).
  // `block: "nearest"` so a small move nudges rather than recentering the whole list.
  useEffect(() => {
    if (flat.length === 0) return;
    listRef.current?.querySelector(`#${CSS.escape(optionId(active))}`)?.scrollIntoView({ block: "nearest" });
  });

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (flat.length === 0) return;
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        // Wraps, so ↓ from the last result returns to the first — a short list is faster to
        // cycle than to reverse out of.
        setActive((i) => (i + 1) % flat.length);
        break;
      case "ArrowUp":
        e.preventDefault();
        setActive((i) => (i - 1 + flat.length) % flat.length);
        break;
      case "Home":
        e.preventDefault();
        setActive(0);
        break;
      case "End":
        e.preventDefault();
        setActive(flat.length - 1);
        break;
      case "Enter": {
        const item = flat[active];
        if (!item) return;
        e.preventDefault();
        onSelect?.(item);
        break;
      }
      default:
        break;
    }
  };
  return (
    <div
      className={cn(
        "flex w-full flex-col overflow-hidden rounded-lg border border-border bg-popover shadow-lg",
        className,
      )}
    >
      <div className="flex items-center gap-2 border-border border-b px-3">
        <Search className="size-4 shrink-0 text-static-400" aria-hidden />
        <input
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder="Search titles, channels, help…"
          aria-label="Search"
          role="combobox"
          aria-expanded={flat.length > 0}
          // Both are omitted when there is no listbox: `aria-controls` would dangle at an id
          // that is not in the DOM, and `aria-activedescendant` at an option that does not exist.
          aria-controls={flat.length > 0 ? listboxId : undefined}
          aria-autocomplete="list"
          aria-activedescendant={flat.length > 0 ? optionId(active) : undefined}
          className="h-11 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        />
        <kbd className="shrink-0 rounded border border-border bg-static-800 px-1.5 py-0.5 font-mono text-2xs text-static-400">
          ESC
        </kbd>
      </div>

      {/* The `listbox` role exists ONLY while there are options to put in it. An empty listbox
          violates `aria-required-children` (axe, critical) — the role promises `option`
          children — so the empty/idle state is a plain scroll container with a message, and the
          input drops `aria-controls` to match. Caught by the gallery's axe sweep after the unit
          tests passed happily: adding ARIA can introduce a violation as easily as it fixes one. */}
      <div
        ref={listRef}
        {...(empty ? {} : { id: listboxId, role: "listbox", "aria-label": "Search results" })}
        className="max-h-80 overflow-y-auto p-1.5"
      >
        {empty && (
          <p className="px-3 py-6 text-center text-muted-foreground text-sm">
            {query ? `No matches for “${query}”.` : "Type to search your library, TMDB, and channels."}
          </p>
        )}
        {/* Each scope is a `group` with its heading as the accessible name. A listbox may only
            contain `option` (or `group`) children, so the bare <p> headings this used to render
            straight into the list were invalid — a screen reader got a flat run of options with
            the scope names read as loose text. */}
        {groups.map((group) => (
          // biome-ignore lint/a11y/useSemanticElements: the rule suggests <fieldset>, which is for form controls and is not a valid listbox child; role="group" is the ARIA-correct way to section options.
          <div key={group.scope} role="group" aria-labelledby={`${listboxId}-${group.scope}`}>
            <p
              id={`${listboxId}-${group.scope}`}
              className="px-2 pt-2 pb-1 font-mono text-2xs text-static-400 uppercase tracking-wide"
            >
              {SCOPE_LABEL[group.scope]}
            </p>
            {group.items.map((item) => {
              // The option's position in the FLAT sequence, which is what the keyboard walks.
              // Looked up by id rather than tracked with a running counter, so the visual
              // grouping and the keyboard order cannot drift apart.
              const index = flat.findIndex((r) => r.id === item.id);
              const isActive = index === active;
              return (
                <button
                  key={item.id}
                  id={optionId(index)}
                  type="button"
                  role="option"
                  aria-selected={isActive}
                  // -1: the input keeps DOM focus (aria-activedescendant pattern), so options
                  // must not enter the tab sequence. They stay clickable.
                  tabIndex={-1}
                  onClick={() => onSelect?.(item)}
                  // Hovering moves the active option too, so the pointer and the keyboard
                  // cannot disagree about which row Enter would pick.
                  onMouseMove={() => setActive(index)}
                  className={cn(
                    "flex w-full cursor-pointer items-center justify-between gap-3 rounded-md px-2 py-2 text-left text-sm transition-colors",
                    isActive && "bg-static-800",
                  )}
                >
                  <span className="min-w-0 truncate">{item.name}</span>
                  <span className="flex shrink-0 items-center gap-2">
                    {item.meta && <span className="font-mono text-static-400 text-xs">{item.meta}</span>}
                    {item.inLibrary && <Badge variant="lock">In library</Badge>}
                  </span>
                </button>
              );
            })}
          </div>
        ))}
      </div>
    </div>
  );
};

export { SearchCommand };
