import * as libraryApi from "@loomarr/api/endpoints/library";
import { CollectionDTOImportedFrom } from "@loomarr/api/models/collectionDTOImportedFrom";
import type { LibraryItemDTO } from "@loomarr/api/models/libraryItemDTO";
import { unwrap } from "@loomarr/api/unwrap";
import type { LibraryCollectionCard, LibraryCollectionPoster, LibrarySetCard } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";
import { ServedImage } from "@/components/loomarr/served-image";

// COLLECTIONS (#2151 decision 28): the household's own collections and the ready-made sets the
// library has a movie of. Copying a set makes it one of the household's, so the list refetches.

const PILE_SIZES = "96px";

const SERVER_NAMES: Record<CollectionDTOImportedFrom, string> = {
  [CollectionDTOImportedFrom.emby]: "Emby",
  [CollectionDTOImportedFrom.jellyfin]: "Jellyfin",
};

// The first three titles, front first, for the card's pile.
const pileOf = (titles: readonly LibraryItemDTO[]): LibraryCollectionPoster[] =>
  titles.slice(0, 3).map((t) => ({
    name: t.name,
    poster: t.poster ? <ServedImage alt="" image={t.poster} sizes={PILE_SIZES} /> : undefined,
  }));

// Only asks while the Collections tab is open.
const useLibraryCollections = (enabled = true) => {
  const queryClient = useQueryClient();
  const list = libraryApi.useListCollections({ query: { enabled } });
  const body = unwrap(list.data);

  const collections = useMemo<LibraryCollectionCard[]>(
    () =>
      (body?.collections ?? []).map((c) => ({
        id: c.id,
        name: c.name,
        titles: c.titles.length,
        pile: pileOf(c.titles),
        importedFrom: c.importedFrom ? SERVER_NAMES[c.importedFrom] : undefined,
        usedBy: c.usedBy.map((u) => ({ name: u.name, number: u.number })),
      })),
    [body],
  );
  const sets = useMemo<LibrarySetCard[]>(
    () =>
      (body?.sets ?? []).map((s) => ({
        id: s.id,
        name: s.name,
        have: s.titles.length,
        total: s.total,
        pile: pileOf(s.titles),
      })),
    [body],
  );

  // Rejects when it didn't copy, so the card can say so.
  const onCopy = useCallback(
    async (id: string) => {
      const res = await libraryApi.copySet(id);
      if (res.status !== 201) throw new Error("Couldn't copy the set");
      await queryClient.invalidateQueries({ queryKey: libraryApi.getListCollectionsQueryKey() });
    },
    [queryClient],
  );

  return {
    collections,
    sets,
    onCopy,
    loaded: body !== undefined,
    error: list.error,
    refetch: list.refetch,
  };
};

export { useLibraryCollections };
