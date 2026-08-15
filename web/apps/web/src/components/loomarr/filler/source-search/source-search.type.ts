import type { DiscoveredClip, DiscoveredClipStats } from "@loomarr/api";

interface SourceSearchProps {
  /** The server's answer, verbatim (contract 1:1). */
  results: DiscoveredClip[];
  /**
   * Runtime + quality, arriving LATE and per row (V35).
   *
   * ⚠ A search deliberately omits these: each one is a `/metadata/<id>` call against
   * archive.org, measured at 22.6s for a page of 25 (median 1.8s each, and slower if more are
   * asked at once). So the row renders immediately with what the search knew, and this fills in
   * underneath as the caller fetches it.
   *
   * ⚠ An id ABSENT from this map is unknown, not zero — a present 0 renders as "0:00" and
   * claims the clip is empty. Absence is what lets the row show nothing instead.
   */
  stats?: Record<string, DiscoveredClipStats>;
  /**
   * Rows currently on screen, so the caller knows which stats to ask for. Called with the full
   * visible set whenever it changes; the caller is expected to de-duplicate against what it has
   * already fetched, because each id costs a real upstream request.
   */
  onVisible?: (ids: string[]) => void;
  /** Ids whose stats are in flight, so a row can say it is still working rather than "—". */
  loadingStats?: string[];
  /**
   * The FULL result count, which is not `results.length` — the server caps a page at 25. An
   * operator judging whether a search is worth pursuing needs the real number.
   */
  total?: number;
  query: string;
  onQueryChange: (q: string) => void;
  onSearch: () => void;
  /**
   * Queue one result for download. ⚠ This is the ONLY path on which anything is fetched:
   * searching streams previews from archive.org and downloads nothing, which the footnote says
   * out loud because the distinction is not visible from the rows.
   */
  onQueue: (clip: DiscoveredClip) => void;
  /** Ids already queued this session, so a row reports `queued ✓` instead of inviting a repeat. */
  queued?: string[];
  /** A queue request is in flight for this id. */
  queueing?: string | null;
  searching?: boolean;
  error?: string | null;
  className?: string;
}

export type { SourceSearchProps };
