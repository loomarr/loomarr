import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { Proposal } from "@loomarr/api/models/proposal";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { ProposalJourneyProposalDTO } from "@loomarr/api/models/proposalJourneyProposalDTO";
import { provisionKey } from "@loomarr/core/provision";
import { useCallback, useEffect, useState } from "react";
import { isSuggestionExpansion, MAX_SUGGESTION_OPTIONS } from "../suggestion-expansion";

const keyFor = (jobId: string) => `loomarr.proposalReviewEdit.${jobId}`;
const stateKeyFor = (jobId: string) => `loomarr.proposalReviewState.${jobId}`;

interface ReviewState {
  version: 2;
  proposalId: string;
  selected: ProposalItem[];
  excluded: string[];
  suggestions: ProposalItem[];
  lastExpansionAdded?: number;
}

interface LegacyReviewState {
  version: 1;
  proposalId: string;
  selected: ProposalItem[];
  excluded: string[];
}

const proposalItems = (items: ProposalItem[] | null | undefined) => items ?? [];

const uniqueItems = (items: ProposalItem[]) => {
  const seen = new Set<string>();
  return items.filter((item) => {
    const key = provisionKey(item);
    const identity = key || `${item.mediaType}:${item.name.toLocaleLowerCase()}:${item.year ?? ""}`;
    if (seen.has(identity)) return false;
    seen.add(identity);
    return true;
  });
};

const normalizeEdit = (edit: ApprovalEditDTO, proposal: Proposal): ApprovalEditDTO | undefined => {
  const selectedKeys = new Set(
    [...proposalItems(proposal.lineup), ...proposalItems(proposal.acquisitions)].map(provisionKey),
  );
  const alternateKeys = new Set(proposalItems(proposal.alternates).map(provisionKey));
  const proposalKeys = new Set([...selectedKeys, ...alternateKeys]);
  const originallyAddedKeys = new Set((edit.add ?? []).map(provisionKey));
  const add = edit.add?.filter((item) => !selectedKeys.has(provisionKey(item)));
  const drop = (edit.drop ?? []).filter(
    (key) => proposalKeys.has(key) && !(selectedKeys.has(key) && originallyAddedKeys.has(key)),
  );

  for (const item of add ?? []) {
    const key = provisionKey(item);
    if (alternateKeys.has(key) && !drop.includes(key)) drop.push(key);
  }

  const note = edit.note?.trim();
  if (drop.length === 0 && (add?.length ?? 0) === 0 && !note) return undefined;
  return {
    ...(drop.length ? { drop } : {}),
    ...(add?.length ? { add } : {}),
    ...(note ? { note } : {}),
  };
};

const selectedItems = (proposal: Proposal, edit?: ApprovalEditDTO) => {
  const dropped = new Set(edit?.drop ?? []);
  return uniqueItems([
    ...proposalItems(proposal.lineup).filter((item) => !dropped.has(provisionKey(item))),
    ...proposalItems(proposal.acquisitions).filter((item) => !dropped.has(provisionKey(item))),
    ...(edit?.add ?? []),
  ]);
};

const initialReviewState = (
  proposal: ProposalJourneyProposalDTO,
  edit?: ApprovalEditDTO,
): { edit?: ApprovalEditDTO; review: ReviewState } => {
  const normalized = edit ? normalizeEdit(edit, proposal.proposal) : undefined;
  const added = new Set((normalized?.add ?? []).map(provisionKey));
  return {
    edit: normalized,
    review: {
      version: 2,
      proposalId: proposal.id,
      selected: selectedItems(proposal.proposal, normalized),
      excluded: (normalized?.drop ?? []).filter((key) => !added.has(key)),
      suggestions: uniqueItems(proposalItems(proposal.proposal.alternates)).slice(0, MAX_SUGGESTION_OPTIONS),
    },
  };
};

const reconcileReplacement = (
  review: ReviewState,
  proposal: ProposalJourneyProposalDTO,
  note?: string,
): { edit?: ApprovalEditDTO; review: ReviewState } => {
  const rawSelected = [
    ...proposalItems(proposal.proposal.lineup),
    ...proposalItems(proposal.proposal.acquisitions),
  ];
  const rawSelectedKeys = new Set(rawSelected.map(provisionKey));
  const alternateKeys = new Set(proposalItems(proposal.proposal.alternates).map(provisionKey));
  const selectedKeys = new Set(review.selected.map(provisionKey));
  const proposalKeys = new Set([...rawSelectedKeys, ...alternateKeys]);
  const drop = uniqueItems(rawSelected)
    .map(provisionKey)
    .filter((key) => key !== "" && !selectedKeys.has(key));

  for (const key of review.excluded) {
    if (proposalKeys.has(key) && !drop.includes(key)) drop.push(key);
  }
  for (const item of review.selected) {
    const key = provisionKey(item);
    if (alternateKeys.has(key) && !drop.includes(key)) drop.push(key);
  }

  const add = review.selected.filter((item) => !rawSelectedKeys.has(provisionKey(item)));
  const trimmedNote = note?.trim();
  const edit =
    drop.length || add.length || trimmedNote
      ? {
          ...(drop.length ? { drop } : {}),
          ...(add.length ? { add } : {}),
          ...(trimmedNote ? { note: trimmedNote } : {}),
        }
      : undefined;
  const eligibleSuggestion = (item: ProposalItem) => {
    const key = provisionKey(item);
    return key !== "" && !selectedKeys.has(key) && !review.excluded.includes(key);
  };
  const expansion = isSuggestionExpansion(proposal.proposal.intent.refineText);
  const retainedSuggestions = expansion ? uniqueItems(review.suggestions).filter(eligibleSuggestion) : [];
  const retainedKeys = new Set(retainedSuggestions.map(provisionKey));
  const suggestions = uniqueItems([
    ...retainedSuggestions,
    ...rawSelected,
    ...proposalItems(proposal.proposal.alternates),
  ])
    .filter(eligibleSuggestion)
    .slice(0, MAX_SUGGESTION_OPTIONS);
  const expansionAdded = suggestions.filter((item) => !retainedKeys.has(provisionKey(item))).length;

  return {
    edit,
    review: {
      ...review,
      proposalId: proposal.id,
      suggestions,
      lastExpansionAdded: expansion ? expansionAdded : undefined,
    },
  };
};

const readEdit = (jobId?: string): ApprovalEditDTO | undefined => {
  if (!jobId || typeof window === "undefined") return undefined;
  try {
    const value = JSON.parse(
      window.sessionStorage.getItem(keyFor(jobId)) ?? "null",
    ) as ApprovalEditDTO | null;
    return value && typeof value === "object" ? value : undefined;
  } catch {
    window.sessionStorage.removeItem(keyFor(jobId));
    return undefined;
  }
};

const readReviewState = (jobId?: string): ReviewState | undefined => {
  if (!jobId || typeof window === "undefined") return undefined;
  try {
    const value = JSON.parse(window.sessionStorage.getItem(stateKeyFor(jobId)) ?? "null") as
      | ReviewState
      | LegacyReviewState
      | null;
    if (!value || !Array.isArray(value.selected) || !Array.isArray(value.excluded)) return undefined;
    if (value.version === 2) {
      return { ...value, suggestions: Array.isArray(value.suggestions) ? value.suggestions : [] };
    }
    if (value.version === 1) {
      return {
        version: 2,
        proposalId: "",
        selected: value.selected,
        excluded: value.excluded,
        suggestions: [],
      };
    }
    return undefined;
  } catch {
    window.sessionStorage.removeItem(stateKeyFor(jobId));
    return undefined;
  }
};

const persist = (jobId: string | undefined, edit: ApprovalEditDTO | undefined, review?: ReviewState) => {
  if (!jobId || typeof window === "undefined") return;
  if (edit) window.sessionStorage.setItem(keyFor(jobId), JSON.stringify(edit));
  else window.sessionStorage.removeItem(keyFor(jobId));
  if (review) window.sessionStorage.setItem(stateKeyFor(jobId), JSON.stringify(review));
  else window.sessionStorage.removeItem(stateKeyFor(jobId));
};

// Pending title choices are part of the resumable review, but never server state:
// this session-scoped delta follows the same durable Job id and still reaches the
// backend only as a parameter to the existing approval gate.
const useProposalReviewEdit = (jobId?: string, proposal?: ProposalJourneyProposalDTO) => {
  const [snapshot, setSnapshot] = useState<{
    jobId?: string;
    edit?: ApprovalEditDTO;
    review?: ReviewState;
  }>(() => ({
    jobId,
    edit: readEdit(jobId),
    review: readReviewState(jobId),
  }));

  useEffect(() => {
    if (snapshot.jobId !== jobId) {
      setSnapshot({ jobId, edit: readEdit(jobId), review: readReviewState(jobId) });
      return;
    }
    if (!proposal) return;
    const next = snapshot.review
      ? snapshot.review.proposalId === proposal.id
        ? { edit: snapshot.edit, review: snapshot.review }
        : reconcileReplacement(snapshot.review, proposal, snapshot.edit?.note)
      : initialReviewState(proposal, snapshot.edit);
    if (JSON.stringify(next.edit) === JSON.stringify(snapshot.edit) && next.review === snapshot.review)
      return;
    persist(jobId, next.edit, next.review);
    setSnapshot({ jobId, ...next });
  }, [jobId, proposal, snapshot]);

  const setEdit = useCallback(
    (edit: ApprovalEditDTO | undefined) => {
      if (!edit) {
        persist(jobId, undefined, undefined);
        setSnapshot({ jobId });
        return;
      }
      const normalized = proposal ? normalizeEdit(edit, proposal.proposal) : edit;
      const previousSelected = new Set(snapshot.review?.selected.map(provisionKey) ?? []);
      const selected = proposal
        ? selectedItems(proposal.proposal, normalized)
        : (snapshot.review?.selected ?? []);
      const selectedKeys = new Set(selected.map(provisionKey));
      const excluded = new Set(snapshot.review?.excluded ?? []);
      for (const key of previousSelected) {
        if (key && !selectedKeys.has(key)) excluded.add(key);
      }
      for (const key of selectedKeys) excluded.delete(key);
      const review = proposal
        ? {
            version: 2 as const,
            proposalId: proposal.id,
            selected,
            excluded: [...excluded],
            suggestions: (snapshot.review?.suggestions ?? []).filter((item) => {
              const key = provisionKey(item);
              return !selectedKeys.has(key) && !excluded.has(key);
            }),
          }
        : snapshot.review;
      persist(jobId, normalized, review);
      setSnapshot({ jobId, edit: normalized, review });
    },
    [jobId, proposal, snapshot.review],
  );

  return [
    snapshot.jobId === jobId ? snapshot.edit : undefined,
    setEdit,
    snapshot.jobId === jobId ? (snapshot.review?.suggestions ?? []) : [],
    snapshot.jobId === jobId ? snapshot.review?.lastExpansionAdded : undefined,
  ] as const;
};

export { useProposalReviewEdit };
