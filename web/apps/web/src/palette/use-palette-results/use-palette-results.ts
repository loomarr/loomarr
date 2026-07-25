import { channelsApi, fillerApi, helpApi, searchApi } from "@loomarr/api";
import type { SearchResult } from "@loomarr/core";

// usePaletteResults fans out across the corpora the ⌘K palette spans (§12: "over
// /v1/search scopes + channels + help").
//
// There is deliberately NO federated entity-search endpoint. /v1/search covers library +
// TMDB only, because a Candidate models a provisionable TITLE — clips and channels are
// not titles, and forcing them through it would push non-titles into the same identity
// machinery that grounds the LLM (§7.2, the leak §10 exists to prevent). So the palette
// merges four sources client-side, which is correct at household scale: ≤50 channels, a
// handful of help pages, and a clip search the store already indexes.
const usePaletteResults = (query: string) => {
  const enabled = query.trim().length > 1;

  // Library + TMDB, the one genuinely federated call.
  const titles = searchApi.useSearch({ q: query, limit: 8 }, { query: { enabled } });
  // Clips live on the filler endpoint, which returns real ClipDTOs — so a palette hit
  // carries a Tunarr program id and can deep-link, which a title-shaped result could not.
  const clips = fillerApi.useListFiller({ q: query }, { query: { enabled } });
  // Channels and help are small, fully-loaded lists: filtered in memory rather than
  // asking the server for a substring match it has no index for.
  const channels = channelsApi.useListChannels({ query: { enabled } });
  const docs = helpApi.useListDocs({ query: { enabled } });

  const q = query.trim().toLowerCase();
  const results: SearchResult[] = [];

  if (channels.data?.status === 200) {
    for (const c of channels.data.data.channels ?? []) {
      if (c.name.toLowerCase().includes(q)) {
        results.push({ id: c.id, scope: "channels", name: c.name, meta: `Channel ${c.number}` });
      }
    }
  }

  if (titles.data?.status === 200) {
    for (const cand of titles.data.data.candidates ?? []) {
      results.push({
        id: cand.tmdbId ? String(cand.tmdbId) : cand.name,
        // in-library vs discover is the distinction that matters before you click:
        // one plays now, the other becomes an acquisition behind the approval gate.
        scope: cand.inLibrary ? "library" : "tmdb",
        name: cand.name,
        ...(cand.year ? { meta: String(cand.year) } : {}),
        inLibrary: cand.inLibrary,
      });
    }
  }

  if (clips.data?.status === 200) {
    for (const clip of (clips.data.data.clips ?? []).slice(0, 6)) {
      results.push({
        id: clip.path,
        scope: "clips",
        name: clip.name,
        ...(clip.era ? { meta: `${clip.era}s` } : {}),
      });
    }
  }

  if (docs.data?.status === 200) {
    for (const d of docs.data.data.docs ?? []) {
      if (d.title.toLowerCase().includes(q)) {
        results.push({ id: d.slug, scope: "help", name: d.title, meta: "Help" });
      }
    }
  }

  return {
    results,
    // Only the network-backed corpora can be "loading"; the in-memory filters are not.
    loading: enabled && (titles.isFetching || clips.isFetching),
  };
};

export { usePaletteResults };
