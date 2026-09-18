import type { ProposalJourneyFailureDTO } from "@loomarr/api/models/proposalJourneyFailureDTO";

interface SuggestionFailureCopy {
  title: string;
  message: string;
  guidance: string;
}

const copyByReason = {
  retrieval_unavailable: {
    title: "Title search is unavailable",
    message: "Loomarr couldn't reach the title sources right now.",
    guidance: "Try again in a little while.",
  },
  reference_unreadable: {
    title: "Couldn't read that link",
    message: "Loomarr couldn't use that page to identify titles.",
    guidance: "Check the link, or describe the channel without it.",
  },
  no_catalog_match: {
    title: "No matches yet",
    message: "Loomarr couldn't confidently match any titles to this description.",
    guidance: "Try a broader description or add a few example titles.",
  },
  named_set_unproven: {
    title: "Couldn't confirm that lineup",
    message: "Loomarr couldn't verify which titles belong to the lineup you named.",
    guidance: "Add a few example titles or try a different description.",
  },
  constraints_conflict: {
    title: "Nothing fits all those details",
    message: "The current details rule out every title Loomarr found.",
    guidance: "Edit the description and loosen one requirement.",
  },
  date_semantics_unclear: {
    title: "Clarify the dates",
    message: "Loomarr couldn't tell which dates should limit the lineup.",
    guidance: "Say whether you mean release dates or episode air dates.",
  },
  invalid_tool_calls: {
    title: "Something went wrong",
    message: "Loomarr couldn't finish checking the available titles.",
    guidance: "Try again. Your description is still here.",
  },
  provider_timeout: {
    title: "This is taking too long",
    message: "The AI service didn't respond in time.",
    guidance: "Try again in a moment.",
  },
  provider_unavailable: {
    title: "AI is temporarily unavailable",
    message: "Loomarr couldn't reach the AI service right now.",
    guidance: "Try again later. If this keeps happening, ask an administrator to check AI settings.",
  },
  provider_response_invalid: {
    title: "Something went wrong",
    message: "Loomarr couldn't finish these suggestions.",
    guidance: "Try again. Your description is still here.",
  },
  discovery_budget_exhausted: {
    title: "Search stopped early",
    message: "The search ended before Loomarr found enough good matches.",
    guidance: "Try again. If this keeps happening, add a few example titles.",
  },
  generation_failed: {
    title: "Something went wrong",
    message: "Loomarr couldn't finish this request.",
    guidance: "Try again. Your description is still here.",
  },
} satisfies Record<ProposalJourneyFailureDTO["reason"], SuggestionFailureCopy>;

const suggestionFailureCopy = (failure: ProposalJourneyFailureDTO): SuggestionFailureCopy =>
  copyByReason[failure.reason];

export type { SuggestionFailureCopy };
export { suggestionFailureCopy };
