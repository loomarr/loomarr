import type { ProposalJourneyFailureDTO } from "@loomarr/api/models/proposalJourneyFailureDTO";
import { describe, expect, it } from "vitest";
import { suggestionFailureCopy } from "./suggestion-failure-copy";

const reasons: ProposalJourneyFailureDTO["reason"][] = [
  "retrieval_unavailable",
  "reference_unreadable",
  "no_catalog_match",
  "named_set_unproven",
  "constraints_conflict",
  "date_semantics_unclear",
  "invalid_tool_calls",
  "provider_timeout",
  "provider_unavailable",
  "provider_response_invalid",
  "discovery_budget_exhausted",
  "generation_failed",
];

describe("suggestionFailureCopy", () => {
  it.each(reasons)("turns %s into fixed plain-language recovery copy", (reason) => {
    const copy = suggestionFailureCopy({
      code: reason === "discovery_budget_exhausted" ? "budget_exhausted" : "generation_failed",
      reason,
      recoveryAction: reason === "reference_unreadable" ? "edit_reference" : "retry_later",
      message: "Provider returned an ungrounded catalog tool failure.",
      guidance: "Inspect bounded generation diagnostics.",
    });

    expect(copy.title).not.toBe("");
    expect(copy.message).not.toBe("");
    expect(copy.guidance).not.toBe("");
    expect(`${copy.title} ${copy.message} ${copy.guidance}`).not.toMatch(
      /\b(?:bounded|catalog|generation|grounded|provider|semantics|tool)\b/i,
    );
  });

  it("tells a person what to change when no titles match", () => {
    const copy = suggestionFailureCopy({
      code: "no_grounded_titles",
      reason: "no_catalog_match",
      recoveryAction: "broaden_request",
      message: "No grounded titles matched this request.",
      guidance: "Broaden the request.",
    });

    expect(copy).toEqual({
      title: "No matches yet",
      message: "Loomarr couldn't confidently match any titles to this description.",
      guidance: "Try a broader description or add a few example titles.",
    });
  });
});
