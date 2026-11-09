import * as libraryApi from "@loomarr/api/endpoints/library";
import type { LibraryCandidateDTO } from "@loomarr/api/models/libraryCandidateDTO";
import type { LibraryNeedDTO } from "@loomarr/api/models/libraryNeedDTO";
import type { LibraryProblemDTO } from "@loomarr/api/models/libraryProblemDTO";
import { unwrap } from "@loomarr/api/unwrap";
import type {
  LibraryNeed,
  LibraryNeedCandidate,
  LibraryNeedSearch,
  LibraryNeedsAnswers,
  LibraryProblem,
} from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";
import { ServedImage } from "@/components/loomarr/served-image";

// NEEDS YOU (#2151 decision 27): the titles Loomarr can't tell what they are, with its guesses, and
// the household's answers. An answer refreshes the list, the status (its count is the nav badge) and
// the titles, since a linked title joins the shelf under its new name and an ignored one leaves it.

const THUMB_SIZES = "144px";

// A title that needs a look (decision 24): Needs you words it from these counts.
const problemOf = (p: LibraryProblemDTO): LibraryProblem => ({
  id: p.item.id,
  kind: p.item.kind,
  name: p.item.name,
  poster: p.item.poster ? <ServedImage alt="" image={p.item.poster} sizes={THUMB_SIZES} /> : undefined,
  unplayable: p.item.unplayable,
  extraBytes: p.extraBytes,
});

const candidateOf = (c: LibraryCandidateDTO): LibraryNeedCandidate => ({
  externalId: c.externalId,
  kind: c.kind,
  name: c.name,
  year: c.year,
  overview: c.overview,
  poster: c.poster ? <ServedImage alt="" image={c.poster} sizes={THUMB_SIZES} /> : undefined,
  nameMatches: c.nameMatches,
  yearMatches: c.yearMatches,
  lengthMatches: c.lengthMatches,
  episodesMatches: c.episodesMatches,
});

// The best guess is the first, when its name agrees; the rest are the others to pick from.
const needOf = (n: LibraryNeedDTO): LibraryNeed => {
  const candidates = n.candidates.map(candidateOf);
  const best = candidates[0]?.nameMatches ? candidates[0] : undefined;
  return {
    id: n.item.id,
    kind: n.item.kind,
    files: n.files.map((f) => f.path),
    best,
    others: best ? candidates.slice(1) : candidates,
  };
};

// A search for a title by name, for "Pick a different title" and "Wrong title?".
const searchLibraryTitles: LibraryNeedSearch = async (query, kind) => {
  const body = unwrap(await libraryApi.searchLibraryTitles({ q: query, kind }));
  if (!body) throw new Error("Couldn't search");
  return body.results.map((r) => ({
    externalId: r.externalId,
    kind: r.kind,
    name: r.name,
    year: r.year,
    overview: r.overview,
    nameMatches: false,
    yearMatches: false,
    lengthMatches: false,
    episodesMatches: false,
  }));
};

const useLibraryNeeds = () => {
  const queryClient = useQueryClient();
  const list = libraryApi.useListLibraryNeeds();
  const needs = useMemo(() => (unwrap(list.data)?.needs ?? []).map(needOf), [list.data]);
  const problems = useMemo(() => (unwrap(list.data)?.problems ?? []).map(problemOf), [list.data]);

  const refresh = useCallback(
    () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: libraryApi.getListLibraryNeedsQueryKey() }),
        queryClient.invalidateQueries({ queryKey: libraryApi.getLibraryStatusQueryKey() }),
        queryClient.invalidateQueries({ queryKey: ["/v1/library/items"] }),
      ]),
    [queryClient],
  );
  const link = libraryApi.useLinkLibraryItem({ mutation: { onSettled: refresh } });
  const ignore = libraryApi.useIgnoreLibraryItem({ mutation: { onSettled: refresh } });

  // A mutation that answers with an error status still resolves; unwrap turns it into a rejection
  // so the card puts the question back.
  const answers: LibraryNeedsAnswers = {
    onLink: async (id, externalId) => {
      const res = await link.mutateAsync({ id, data: { externalId } });
      if (res.status !== 204) throw new Error("Couldn't save your answer");
    },
    onIgnore: async (id) => {
      const res = await ignore.mutateAsync({ id });
      if (res.status !== 204) throw new Error("Couldn't save your answer");
    },
  };

  return {
    needs,
    problems,
    loaded: list.data !== undefined,
    error: list.error,
    refetch: list.refetch,
    answers,
    onSearch: searchLibraryTitles,
  };
};

export { searchLibraryTitles, useLibraryNeeds };
