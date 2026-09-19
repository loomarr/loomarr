import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { EpisodeSelection } from "@loomarr/api/models/episodeSelection";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { ReactNode } from "react";
import type { MovieCollectionChoice } from "@/suggest/movie-collection-choices";

interface ProposalEditProps {
  // The proposal's two pick lists, as the queue already renders them. Both are needed:
  // dropping an in-library title changes the channel, dropping an acquisition also stops a
  // download, and the approver should see one list of "what I am approving".
  lineup: ProposalItem[];
  acquisitions: ProposalItem[];
  alternates?: ProposalItem[];
  // Successive revisions may contribute grounded picks to the Proposal Job's
  // cumulative option pool. They remain optional until the reviewer adds them.
  optionalSuggestions?: ProposalItem[];
  onFindMore?: () => void;
  findingMore?: boolean;
  movieCollections?: MovieCollectionChoice[];
  movieCollectionsLoading?: boolean;
  movieCollectionsIncomplete?: boolean;
  // Read-only server projection from the trusted Proposal Intent. Used only to
  // explain series added in this local edit; it is never copied into the edit DTO.
  episodeSelectionPreview?: EpisodeSelection;
  // Pass this prop (including explicit undefined) to control the pending delta from the
  // workspace. Omit it only for a self-contained/uncontrolled editor.
  value?: ApprovalEditDTO;
  // Called on every change with the pending edit — or `undefined` when nothing has been modified.
  // The note intentionally remains untrimmed while the user types; the approval boundary owns
  // normalization. Undefined is not the same as an empty edit (see the component): the caller
  // must send NO body in that case, so an unmodified approval stays byte-identical to what it
  // was before edit-before-approve existed.
  onChange?: (edit: ApprovalEditDTO | undefined) => void;
  renderFeedback?: (item: ProposalItem) => ReactNode;
  showNote?: boolean;
  disabled?: boolean;
  className?: string;
}

export type { ProposalEditProps };
