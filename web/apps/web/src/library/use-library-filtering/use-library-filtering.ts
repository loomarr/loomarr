import * as libraryApi from "@loomarr/api/endpoints/library";
import type { ListLibraryItemsKind } from "@loomarr/api/models/listLibraryItemsKind";
import { unwrap } from "@loomarr/api/unwrap";
import type { LibraryShelfFiltering } from "@loomarr/ui";
import { keepPreviousData } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { LibraryItemsFilter } from "../library-items-query";

// The wall's filters past the genre (#4476): the choices the tab offers, the person search and the
// chosen person's name. The filters themselves are the URL's; the page owns changing them.

type LibraryUrlFilters = Pick<
  LibraryItemsFilter,
  "studio" | "person" | "audio" | "subtitles" | "uhd" | "hdr"
>;

const PERSON_SETTLE_MS = 250;
const PEOPLE_LIMIT = 8;
// One character matches most of the credits, so the search waits for a second.
const PEOPLE_MIN_QUERY = 2;

const useLibraryFiltering = ({
  enabled,
  kind,
  filters,
}: {
  enabled: boolean;
  kind?: ListLibraryItemsKind;
  filters: LibraryUrlFilters;
}): Pick<LibraryShelfFiltering, "facets" | "personName" | "personSearch"> => {
  const [typed, setTyped] = useState("");
  const query = typed.trim();
  const [settled, setSettled] = useState("");
  useEffect(() => {
    const t = setTimeout(() => setSettled(query), PERSON_SETTLE_MS);
    return () => clearTimeout(t);
  }, [query]);

  const facets = libraryApi.useListLibraryFacets({ kind }, { query: { enabled } });
  const people = libraryApi.useSearchLibraryPeople(
    { q: settled, kind, limit: PEOPLE_LIMIT },
    { query: { enabled: enabled && settled.length >= PEOPLE_MIN_QUERY, placeholderData: keepPreviousData } },
  );
  // A link carrying a person arrives with only the id.
  const person = libraryApi.useGetLibraryPerson(filters.person ?? "", {
    query: { enabled: enabled && Boolean(filters.person) },
  });

  const searching = query.length >= PEOPLE_MIN_QUERY;
  const found = searching ? (unwrap(people.data)?.people ?? []) : [];
  return {
    facets: unwrap(facets.data),
    personName: filters.person ? unwrap(person.data)?.name : undefined,
    personSearch: {
      value: typed,
      onChange: setTyped,
      options: found.map(({ id, name, titles }) => ({
        value: id,
        label: name,
        trailing: titles.toLocaleString("en-US"),
      })),
      loading: searching && (query !== settled || people.isFetching),
      failed: searching && query === settled && people.isError,
    },
  };
};

export type { LibraryUrlFilters };
export { useLibraryFiltering };
