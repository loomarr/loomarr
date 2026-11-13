import { Action, Field, Surface, Text } from "@loomarr/design-system";
import { useEffect, useState } from "react";
import type { LibraryNeedCandidate, LibraryNeedSearch } from "../library-needs/library-needs.type";
import { LibraryPoster, Line, Stack } from "../library-poster";
import type { LibraryKind } from "../library-shelf/library-shelf.type";

// "Pick a different title" (#2151 decisions 26–27): the other guesses, or what a search for a typed
// name finds, each with "Use this one". Needs you and a title's "Wrong title?" both open it.

const SEARCH_SETTLE_MS = 300;

const nameYear = (c: LibraryNeedCandidate) => (c.year ? `${c.name} (${c.year})` : c.name);

// The search as typed, settled for a moment before it is asked.
const useSettled = (value: string) => {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setSettled(value), SEARCH_SETTLE_MS);
    return () => clearTimeout(t);
  }, [value]);
  return settled;
};

type Found = { query: string; results?: readonly LibraryNeedCandidate[]; failed?: boolean };

const PickTitle = ({
  kind,
  others,
  onSearch,
  onPick,
}: {
  kind: LibraryKind;
  /** Shown before anything is typed. */
  others: readonly LibraryNeedCandidate[];
  onSearch: LibraryNeedSearch;
  onPick: (c: LibraryNeedCandidate) => void;
}) => {
  const [search, setSearch] = useState("");
  const query = useSettled(search.trim());
  const [found, setFound] = useState<Found>({ query: "" });
  useEffect(() => {
    if (query === "") return;
    let current = true;
    onSearch(query, kind).then(
      (results) => current && setFound({ query, results }),
      () => current && setFound({ query, failed: true }),
    );
    return () => {
      current = false;
    };
  }, [query, kind, onSearch]);
  const searched = query !== "" && found.query === query;
  const list = query === "" ? others : searched ? (found.results ?? []) : [];
  return (
    <Surface gap="inline" padding="control">
      <Field
        label="Search for the title"
        onChangeText={setSearch}
        placeholder="Type a movie or show name"
        value={search}
      />
      {searched && found.failed ? (
        <Text textRole="body" tone="secondary">
          Couldn't search just now. Try again in a moment.
        </Text>
      ) : searched && list.length === 0 ? (
        <Text textRole="body" tone="secondary">{`Nothing matches "${query}".`}</Text>
      ) : null}
      {list.map((c) => (
        <Line flexWrap="nowrap" key={`${c.externalId.namespace}:${c.externalId.value}`}>
          <Stack flexShrink={0} width={28}>
            <LibraryPoster art={c.poster} name={c.poster ? c.name : ""} />
          </Stack>
          <Stack flexBasis={0} flexGrow={1} flexShrink={1} minWidth={0}>
            <Text textRole="body">{nameYear(c)}</Text>
          </Stack>
          <Action accessibilityLabel={`Use ${nameYear(c)}`} onPress={() => onPick(c)} variant="outline">
            Use this one
          </Action>
        </Line>
      ))}
    </Surface>
  );
};

export { nameYear, PickTitle };
