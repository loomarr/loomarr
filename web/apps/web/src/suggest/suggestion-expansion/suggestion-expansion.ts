import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { Intent } from "@loomarr/api/models/intent";
import type { LineupContext } from "@loomarr/api/models/lineupContext";
import type { Proposal } from "@loomarr/api/models/proposal";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { provisionKey } from "@loomarr/core/provision";

const MAX_SUGGESTION_OPTIONS = 24;
const SUGGESTION_EXPANSION_PREFIX = "Find 6–8 additional titles that match this brief.";
const SUGGESTION_EXPANSION_REQUEST = `${SUGGESTION_EXPANSION_PREFIX} Do not repeat or replace the selected lineup.`;

const suggestionIdentity = (item: ProposalItem) => {
  const key = provisionKey(item);
  return key || `${item.mediaType}:${item.name.toLocaleLowerCase()}:${item.year ?? ""}`;
};

const isSuggestionExpansion = (refineText?: string) =>
  refineText?.startsWith(SUGGESTION_EXPANSION_PREFIX) ?? false;

const suggestionExpansionRequest = (items: ProposalItem[]) => {
  const seen = new Set<string>();
  const shown = items.flatMap((item) => {
    const identity = suggestionIdentity(item);
    if (seen.has(identity)) return [];
    seen.add(identity);
    return [`${JSON.stringify(item.name)}${item.year ? ` (${item.year})` : ""}`];
  });
  return shown.length > 0
    ? `${SUGGESTION_EXPANSION_REQUEST} Do not return these suggestions already shown: ${shown.join("; ")}.`
    : SUGGESTION_EXPANSION_REQUEST;
};

// The titles a review currently keeps — the proposal's lineup and acquisitions minus what the
// edit dropped, plus what it added — as the LineupContext an expansion request carries.
const selectedLineupContext = (proposal: Proposal, edit?: ApprovalEditDTO): LineupContext[] => {
  const dropped = new Set(edit?.drop ?? []);
  const selected = [
    ...(proposal.lineup ?? []).filter((item) => !dropped.has(provisionKey(item))),
    ...(proposal.acquisitions ?? []).filter((item) => !dropped.has(provisionKey(item))),
    ...(edit?.add ?? []),
  ];
  const seen = new Set<string>();

  return selected.flatMap((item) => {
    const key = provisionKey(item);
    const identity = suggestionIdentity(item);
    if (seen.has(identity)) return [];
    seen.add(identity);
    return [
      {
        name: item.name,
        ...(item.year ? { year: item.year } : {}),
        ...(key ? { key } : {}),
      },
    ];
  });
};

// The "find more" revision: the same request, the lineup the review keeps, and the expansion
// refinement. The server prompt treats it as option expansion — the kept lineup is preserved
// separately and only new titles come back, which the review then offers as optional picks.
const suggestionExpansionIntent = (
  proposal: Proposal,
  edit: ApprovalEditDTO | undefined,
  shown: ProposalItem[],
  description = proposal.intent.description,
): Intent => {
  const { refineText: _previousRefinement, currentLineup: _previousLineup, ...request } = proposal.intent;
  return {
    ...request,
    description,
    currentLineup: selectedLineupContext(proposal, edit),
    refineText: suggestionExpansionRequest(shown),
  };
};

export {
  isSuggestionExpansion,
  MAX_SUGGESTION_OPTIONS,
  selectedLineupContext,
  suggestionExpansionIntent,
  suggestionExpansionRequest,
  suggestionIdentity,
};
