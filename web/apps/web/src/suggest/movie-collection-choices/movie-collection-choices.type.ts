import type { SearchCandidate } from "@loomarr/api/models/searchCandidate";

interface MovieCollectionChoice {
  tmdbId: number;
  name: string;
  members: SearchCandidate[];
}

interface MovieCollectionChoicesProps {
  collections: MovieCollectionChoice[];
  selectedKeys: ReadonlySet<string>;
  editable: boolean;
  disabled?: boolean;
  loading?: boolean;
  incomplete?: boolean;
  onAddCollection: (collection: MovieCollectionChoice) => void;
  onAddMember: (member: SearchCandidate) => void;
}

export type { MovieCollectionChoice, MovieCollectionChoicesProps };
