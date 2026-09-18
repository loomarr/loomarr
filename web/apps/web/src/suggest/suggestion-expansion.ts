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

export { isSuggestionExpansion, MAX_SUGGESTION_OPTIONS, suggestionExpansionRequest, suggestionIdentity };
