import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { writeSuggestionDraft } from "../suggestion-draft";
import { SuggestionDraftReturn } from "./suggestion-draft-return";

describe("SuggestionDraftReturn", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("stays absent when there is no saved channel intent", () => {
    render(<RouterHarness content={<SuggestionDraftReturn />} />);
    expect(screen.queryByText(/channel draft is saved/i)).not.toBeInTheDocument();
  });

  it("links back to the Guide when a draft is saved", async () => {
    writeSuggestionDraft({ description: "Saturday morning cartoons" });
    render(<RouterHarness content={<SuggestionDraftReturn />} />);

    expect(await screen.findByText(/channel draft is saved/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /return to channel/i })).toHaveAttribute("href", "/guide");
  });
});
