// Shared component *data contracts* (frontend-design §4.2). These are the platform-
// agnostic shapes the UI renders — mirrors of the API DTOs (internal/*), kept here so
// the web components, the `packages/fixtures` "test card" data, and the future Expo/RN
// components all type against ONE definition. Component *prop* interfaces (with
// handlers/className) stay per-platform; only the data shapes are shared.

// Filler clip — mirrors @loomarr/api ClipDTO (internal/provision, §10).
type ClipKind = "commercial" | "bumper" | "station_id" | "psa" | "trailer" | "interstitial";
type ClipAudience = "kids" | "family" | "general" | "late_night";

interface Clip {
  name: string;
  kind: ClipKind;
  durationMs: number;
  era?: number;
  audience?: ClipAudience;
  category?: string;
  tagged: boolean;
  aiTagged: boolean;
  source?: string;
}

// Suggester proposal — mirrors suggest.Proposal (internal/suggest/proposal.go); the API
// ships it as opaque JSON, so this is the view shape the UI reasons about (§8).
type ProposalMediaType = "movie" | "series";

interface ProposalItemView {
  name: string;
  year?: number;
  mediaType: ProposalMediaType;
  inLibrary: boolean;
  rationale?: string;
  confidence?: number;
  seasons?: number[];
}

interface ProposalScores {
  themeFit: number;
  availabilityRatio: number;
}

interface ProposalView {
  rationale?: string;
  lineup: ProposalItemView[];
  acquisitions: ProposalItemView[];
  alternates: ProposalItemView[];
  scores?: ProposalScores;
}

// ⌘K search — mirrors @loomarr/api SearchCandidate + the palette scopes (§7.2).
type SearchScope = "library" | "tmdb" | "clips" | "channels" | "help";

interface SearchResult {
  id: string;
  scope: SearchScope;
  name: string;
  meta?: string;
  inLibrary?: boolean;
}

export type {
  Clip,
  ClipAudience,
  ClipKind,
  ProposalItemView,
  ProposalMediaType,
  ProposalScores,
  ProposalView,
  SearchResult,
  SearchScope,
};
