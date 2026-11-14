import { Action, Badge, Combobox, type ComboboxGroup, Icon, icons, Text } from "@loomarr/design-system";
import type { PaletteScope, SearchCommandProps, SearchResult } from "./search-command.type";

// SearchCommand — the ⌘K palette (§3, §7.2) over library/TMDB/clips + channels + help.
// In-library results wear a `lock` flag so you see at a glance what's ready now vs.
// what would be an acquisition (§8). Presentational + controlled; the global ⌘K trigger
// lives in `usePaletteShortcut`.
//
// It is the design-system Combobox in `inline` mode: an always-open, grouped list inside a card,
// which is what the v2 mock draws (the ⌘K block in `design/loomarr-prototype-desktop-v2.dc.html`)
// and is why it stayed hand-rolled until Base UI's inline mode existed. The keyboard contract
// (arrows across the group headings with wrap-around, Home and End, Enter, hover moving the
// highlight, `aria-activedescendant` with DOM focus kept in the input) belongs to the Combobox and
// is pinned by its tests, not here. Axe passing never proved any of it: it shipped once with
// arrow keys dead and a green sweep, so a change to the Combobox's keys is verified by pressing them.
//
// Escape is not bound by DEFAULT: the ⌘K palette is a Dialog, which owns the key, so an
// unconditional handler here would close twice. Every scoped picker opts in via `onEscape`, which
// the Combobox handles before its empty-list case, because a picker with no matches is exactly when
// a user wants out.
const SCOPE_LABEL: Record<PaletteScope, string> = {
  diagnostics: "Admin tools",
  channels: "Channels",
  library: "In your library",
  tmdb: "Discover (TMDB)",
  clips: "Filler clips",
  help: "Help",
};

const SCOPE_ORDER: PaletteScope[] = ["diagnostics", "channels", "library", "tmdb", "clips", "help"];

// Two scopes can hand back the same id, so an option's value is the pair.
const optionValue = (result: SearchResult) => `${result.scope}:${result.id}`;

const SearchCommand = ({
  query,
  onQueryChange,
  results,
  onSelect,
  loading = false,
  placeholder = "Search titles, channels, help…",
  onEscape,
  fill = false,
  onClose,
}: SearchCommandProps) => {
  const byValue = new Map(results.map((result) => [optionValue(result), result]));
  const groups: ComboboxGroup[] = SCOPE_ORDER.map((scope) => ({
    id: scope,
    label: SCOPE_LABEL[scope],
    options: results
      .filter((result) => result.scope === scope)
      .map((result) => ({
        label: result.name,
        trailing:
          result.meta || result.inLibrary ? (
            <>
              {result.meta && (
                <Text as="span" textRole="mono">
                  {result.meta}
                </Text>
              )}
              {result.inLibrary && (
                <Badge shape="square" tone="lock">
                  In library
                </Badge>
              )}
            </>
          ) : undefined,
        value: optionValue(result),
      })),
  }));

  return (
    <Combobox
      // The first result is always the active one, so Enter picks it before an arrow is pressed.
      autoHighlight="always"
      // Not said while a search is running, so a slow one never flashes "no matches".
      emptyText={
        loading
          ? undefined
          : query
            ? `No matches for “${query}”.`
            : "Type to search your library, TMDB, and channels."
      }
      endAdornment={
        onClose ? (
          <Action density="touch" icon="close" layout="icon" onPress={onClose} variant="ghost">
            Close search
          </Action>
        ) : (
          <kbd className="border-border bg-static-800 text-2xs text-static-400 shrink-0 rounded border px-1.5 py-0.5 font-mono">
            ESC
          </kbd>
        )
      }
      fill={fill}
      groups={groups}
      inline
      inputValue={query}
      label="Search"
      listLabel="Search results"
      onEscape={onEscape}
      onInputValueChange={onQueryChange}
      onSelect={(option) => {
        const result = byValue.get(option.value);
        if (result) onSelect?.(result);
      }}
      placeholder={placeholder}
      startAdornment={<Icon decorative glyph={icons.search} size="compact" tone="muted" />}
    />
  );
};

export { SearchCommand };
