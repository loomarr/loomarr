import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { provisionKey } from "@loomarr/core/provision";

const MAX_MOVIE_COLLECTION_KEYS = 24;

const movieCollectionKeys = (items: readonly ProposalItem[]) => {
  const keys: string[] = [];
  const seen = new Set<string>();
  for (const item of items) {
    if (item.mediaType !== "movie" || !Number.isInteger(item.tmdbId) || (item.tmdbId ?? 0) <= 0) continue;
    const key = provisionKey(item);
    if (!key.startsWith("movie:tmdb:") || seen.has(key)) continue;
    seen.add(key);
    keys.push(key);
    if (keys.length === MAX_MOVIE_COLLECTION_KEYS) break;
  }
  return keys;
};

export { MAX_MOVIE_COLLECTION_KEYS, movieCollectionKeys };
