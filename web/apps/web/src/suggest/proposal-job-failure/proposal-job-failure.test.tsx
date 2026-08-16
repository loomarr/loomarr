import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { ProposalJobFailure } from "./proposal-job-failure";

describe("ProposalJobFailure", () => {
  it("focuses the alert and offers editing for an ungrounded request", async () => {
    const edit = vi.fn();
    const retry = vi.fn();
    render(
      <RouterHarness
        content={
          <ProposalJobFailure
            failure={{ code: "no_grounded_titles", message: "No grounded titles matched." }}
            onEdit={edit}
            onRetry={retry}
          />
        }
      />,
    );
    expect(await screen.findByRole("alert")).toHaveFocus();
    await userEvent.click(screen.getByRole("button", { name: "Edit description" }));
    expect(edit).toHaveBeenCalledOnce();
  });

  it("links provider failures to AI settings and always offers a fresh retry", async () => {
    const retry = vi.fn();
    render(
      <RouterHarness
        content={
          <ProposalJobFailure
            failure={{ code: "provider_unavailable", message: "The provider is unavailable." }}
            onRetry={retry}
          />
        }
      />,
    );
    expect(await screen.findByRole("link", { name: "Open AI settings" })).toHaveAttribute(
      "href",
      "/settings/ai",
    );
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(retry).toHaveBeenCalledOnce();
  });

  it("links an admin's generic generation failure to the LLM diagnostic guide", async () => {
    render(
      <RouterHarness
        content={
          <ProposalJobFailure
            failure={{ code: "generation_failed", message: "Loomarr couldn't generate this channel." }}
            isAdmin
            onRetry={vi.fn()}
          />
        }
      />,
    );

    const diagnostic = await screen.findByRole("link", { name: "Troubleshoot" });
    expect(diagnostic).toHaveAttribute("href", "/help?page=troubleshooting&section=llm");
  });

  it("does not expose the admin diagnostic link to a member", () => {
    render(
      <RouterHarness
        content={
          <ProposalJobFailure
            failure={{ code: "generation_failed", message: "Loomarr couldn't generate this channel." }}
            isAdmin={false}
            onRetry={vi.fn()}
          />
        }
      />,
    );

    expect(screen.queryByRole("link", { name: "Troubleshoot" })).not.toBeInTheDocument();
  });
});
