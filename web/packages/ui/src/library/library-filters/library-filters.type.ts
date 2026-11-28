import type { ComboboxOption } from "@loomarr/design-system";

/** The wall's filters beyond the genre, as the list reads them; an absent one filters nothing. */
interface LibraryShelfFilters {
  studio?: string;
  /** Only titles crediting this person, by id. */
  person?: string;
  /** A language code. */
  audio?: string;
  /** A language code. */
  subtitles?: string;
  uhd?: boolean;
  /** `any`, or one kind of high dynamic range. */
  hdr?: string;
}

/** One choice of a filter: what the list takes, what to call it and how many titles it keeps. */
interface LibraryShelfFacet {
  value: string;
  name: string;
  titles: number;
}

/** The choices the tab's titles offer. */
interface LibraryShelfFacets {
  studios: readonly LibraryShelfFacet[];
  audio: readonly LibraryShelfFacet[];
  subtitles: readonly LibraryShelfFacet[];
  uhd: number;
  hdrAny: number;
}

/** The person search: what is typed, and the people it found. */
interface LibraryShelfPersonSearch {
  value: string;
  onChange: (value: string) => void;
  options: readonly ComboboxOption[];
  /** The people shown belong to an earlier search, or the search is running. */
  loading: boolean;
  failed: boolean;
}

interface LibraryShelfFiltering {
  filters: LibraryShelfFilters;
  /** Sets the filters given; an `undefined` one is cleared. */
  onChange: (patch: LibraryShelfFilters) => void;
  /** Clears every filter, the genre with them. */
  onClear: () => void;
  /** Absent until they arrive. */
  facets?: LibraryShelfFacets;
  /** The chosen person's name; absent while it is read, or when it can't be. */
  personName?: string;
  personSearch: LibraryShelfPersonSearch;
  /** How many titles the list holds with every filter on; absent while it loads. */
  total?: number;
}

export type {
  LibraryShelfFacet,
  LibraryShelfFacets,
  LibraryShelfFiltering,
  LibraryShelfFilters,
  LibraryShelfPersonSearch,
};
