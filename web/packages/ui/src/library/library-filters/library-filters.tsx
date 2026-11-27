import { Action, Combobox, CompactSelect, Text, Toggle, useWindowWidth } from "@loomarr/design-system";

import { Line, Stack } from "../library-poster";
import type { LibraryShelfFacet, LibraryShelfFiltering } from "./library-filters.type";

// The Library wall's filters past the genre: studio, person, audio and subtitle language, 4K and
// HDR. They sit in a disclosure under the controls, and what is on is listed under it, each one
// removable, with one Clear filters. Presentational: the app owns the URL and the requests.

const PHONE = 768;
const ALL = "";

const count = (n: number) => n.toLocaleString("en-US");

// A chosen value the tab's choices don't hold is still listed, so the select shows what filters.
const choices = (all: string, facets: readonly LibraryShelfFacet[], chosen: string | undefined) => {
  const options = [
    { label: all, value: ALL },
    ...facets.map(({ name, titles, value }) => ({ label: `${name} · ${count(titles)}`, value })),
  ];
  if (chosen && !facets.some((f) => f.value === chosen)) options.push({ label: chosen, value: chosen });
  return options;
};

// The server matches a studio or a language in any case; the choice's own spelling is shown.
const listed = (facets: readonly LibraryShelfFacet[] | undefined, value: string | undefined) =>
  facets?.find((f) => f.value.toLowerCase() === value?.toLowerCase());

/** How many of these filters are on; the genre has its own select. */
const filterCount = ({ filters }: LibraryShelfFiltering) =>
  [filters.studio, filters.person, filters.audio, filters.subtitles, filters.uhd, filters.hdr].filter(Boolean)
    .length;

const Select = ({
  label,
  all,
  facets,
  chosen,
  onChoose,
}: {
  label: string;
  all: string;
  facets: readonly LibraryShelfFacet[];
  chosen: string | undefined;
  onChoose: (value: string | undefined) => void;
}) => {
  const wide = useWindowWidth() >= PHONE;
  const value = listed(facets, chosen)?.value ?? chosen ?? ALL;
  return (
    <Stack gap="inline" width={wide ? 220 : "100%"}>
      <Text textRole="compact" tone="secondary">
        {label}
      </Text>
      <CompactSelect
        density={wide ? "pointer" : "touch"}
        label={label}
        onValueChange={(next) => onChoose(next === ALL ? undefined : next)}
        options={choices(all, facets, value || undefined)}
        value={value}
      />
    </Stack>
  );
};

const FilterPanel = ({ filtering }: { filtering: LibraryShelfFiltering }) => {
  const { filters, onChange, facets, personSearch } = filtering;
  const wide = useWindowWidth() >= PHONE;
  const density = wide ? "pointer" : "touch";
  return (
    <Line
      alignItems="flex-end"
      aria-label="More filters"
      flexDirection={wide ? "row" : "column"}
      flexWrap="wrap"
      gap="control"
      role="group"
    >
      <Select
        all="All studios"
        chosen={filters.studio}
        facets={facets?.studios ?? []}
        label="Studio"
        onChoose={(studio) => onChange({ studio })}
      />
      <Stack gap="inline" width={wide ? 260 : "100%"}>
        <Text textRole="compact" tone="secondary">
          Person
        </Text>
        <Combobox
          density={density}
          emptyText={personSearch.value.trim() ? "No one by that name." : undefined}
          errorText={personSearch.failed ? "Couldn't search people. Try again." : undefined}
          inputValue={personSearch.value}
          label="Person"
          loading={personSearch.loading}
          onInputValueChange={personSearch.onChange}
          onSelect={(option) => {
            personSearch.onChange("");
            onChange({ person: option.value });
          }}
          options={personSearch.options}
          placeholder="Search cast and crew"
          stale={personSearch.loading}
        />
      </Stack>
      <Select
        all="Any audio language"
        chosen={filters.audio}
        facets={facets?.audio ?? []}
        label="Audio language"
        onChoose={(audio) => onChange({ audio })}
      />
      <Select
        all="Any subtitles"
        chosen={filters.subtitles}
        facets={facets?.subtitles ?? []}
        label="Subtitle language"
        onChoose={(subtitles) => onChange({ subtitles })}
      />
      <Toggle
        checked={Boolean(filters.uhd)}
        density={density}
        description={facets ? `${count(facets.uhd)} in 4K` : undefined}
        kind="switch"
        label="4K"
        onCheckedChange={(on) => onChange({ uhd: on || undefined })}
      />
      <Toggle
        checked={Boolean(filters.hdr)}
        density={density}
        description={facets ? `${count(facets.hdrAny)} in HDR` : undefined}
        kind="switch"
        label="HDR"
        onCheckedChange={(on) => onChange({ hdr: on ? "any" : undefined })}
      />
    </Line>
  );
};

interface ActiveFilter {
  key: string;
  text: string;
  remove: () => void;
}

const activeFilters = (
  { filters, onChange, facets, personName }: LibraryShelfFiltering,
  genre: string | undefined,
  onGenreClear: () => void,
): ActiveFilter[] => {
  const on: (ActiveFilter | false | undefined | "")[] = [
    filters.person && {
      key: "person",
      text: `Titles with ${personName ?? "this person"}`,
      remove: () => onChange({ person: undefined }),
    },
    genre && { key: "genre", text: genre, remove: onGenreClear },
    filters.studio && {
      key: "studio",
      text: listed(facets?.studios, filters.studio)?.name ?? filters.studio,
      remove: () => onChange({ studio: undefined }),
    },
    filters.audio && {
      key: "audio",
      text: `Audio: ${listed(facets?.audio, filters.audio)?.name ?? filters.audio}`,
      remove: () => onChange({ audio: undefined }),
    },
    filters.subtitles && {
      key: "subtitles",
      text: `Subtitles: ${listed(facets?.subtitles, filters.subtitles)?.name ?? filters.subtitles}`,
      remove: () => onChange({ subtitles: undefined }),
    },
    filters.uhd && { key: "uhd", text: "4K", remove: () => onChange({ uhd: undefined }) },
    filters.hdr && {
      key: "hdr",
      text: filters.hdr === "any" ? "HDR" : filters.hdr,
      remove: () => onChange({ hdr: undefined }),
    },
  ];
  return on.filter((entry): entry is ActiveFilter => Boolean(entry));
};

// What is on, each removable, with how many titles that leaves. With nothing to show, the empty
// panel below carries Clear filters, so this line leaves it out.
const ActiveFilters = ({
  filtering,
  genre,
  onGenreClear,
  clearable,
}: {
  filtering: LibraryShelfFiltering;
  genre: string | undefined;
  onGenreClear: () => void;
  clearable: boolean;
}) => {
  const wide = useWindowWidth() >= PHONE;
  const density = wide ? "pointer" : "touch";
  const on = activeFilters(filtering, genre, onGenreClear);
  if (on.length === 0) return null;
  const { total } = filtering;
  return (
    <Line gap="control">
      {on.map(({ key, text, remove }) => (
        <Action
          accessibilityLabel={`Remove filter: ${text}`}
          density={density}
          icon="close"
          iconPlacement="end"
          key={key}
          onPress={remove}
          variant="outline"
        >
          {text}
        </Action>
      ))}
      {clearable ? (
        <Action density={density} onPress={filtering.onClear} variant="ghost">
          Clear filters
        </Action>
      ) : null}
      {total === undefined ? null : (
        <Stack role="status">
          <Text textRole="compact" tone="secondary">
            {total === 1 ? "1 title" : `${count(total)} titles`}
          </Text>
        </Stack>
      )}
    </Line>
  );
};

export { ActiveFilters, FilterPanel, filterCount };
