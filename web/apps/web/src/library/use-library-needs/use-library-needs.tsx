import * as libraryApi from "@loomarr/api/endpoints/library";
import type { LibraryCandidateDTO } from "@loomarr/api/models/libraryCandidateDTO";
import type { LibraryNeedDTO } from "@loomarr/api/models/libraryNeedDTO";
import { unwrap } from "@loomarr/api/unwrap";
import type { LibraryNeed, LibraryNeedCandidate, LibraryNeedSearch, LibraryNeedsAnswers } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";
import { ServedImage } from "@/components/loomarr/served-image";

// NEEDS YOU (#2151 decision 27): the titles Loomarr can't tell what they are, with its guesses, and
// the household's answers. An answer refreshes the list, the status (its count is the nav badge) and
// the titles, since a linked title joins the shelf under its new name and an ignored one leaves it.

const THUMB_SIZES = "144px";

const candidateOf = (c: LibraryCandidateDTO): LibraryNeedCandidate => ({
  tmdbId: c.tmdbId,
  kind: c.kind,
  name: c.name,
  year: c.year,
  overview: c.overview,
  poster: c.poster ? <ServedImage alt="" image={c.poster} sizes={THUMB_SIZES} /> : undefined,
  nameMatches: c.nameMatches,
  yearMatches: c.yearMatches,
  lengthMatches: c.lengthMatches,
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

const useLibraryNeeds = () => {
  const queryClient = useQueryClient();
  const list = libraryApi.useListLibraryNeeds();
  const needs = useMemo(() => (unwrap(list.data)?.needs ?? []).map(needOf), [list.data]);

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
    onLink: async (id, tmdbId) => {
      const res = await link.mutateAsync({ id, data: { tmdbId } });
      if (res.status !== 204) throw new Error("Couldn't save your answer");
    },
    onIgnore: async (id) => {
      const res = await ignore.mutateAsync({ id });
      if (res.status !== 204) throw new Error("Couldn't save your answer");
    },
  };

  const onSearch: LibraryNeedSearch = useCallback(async (query, kind) => {
    const res = await libraryApi.searchLibraryTitles({ q: query, kind });
    const body = unwrap(res);
    if (!body) throw new Error("Couldn't search");
    return body.results.map((r) => ({
      tmdbId: r.tmdbId,
      kind: r.kind,
      name: r.name,
      year: r.year,
      overview: r.overview,
      nameMatches: false,
      yearMatches: false,
      lengthMatches: false,
    }));
  }, []);

  return {
    needs,
    loaded: list.data !== undefined,
    error: list.error,
    refetch: list.refetch,
    answers,
    onSearch,
  };
};

export { useLibraryNeeds };
