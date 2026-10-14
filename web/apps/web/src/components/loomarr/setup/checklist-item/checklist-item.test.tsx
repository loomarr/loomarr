import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { ChecklistItem } from "./checklist-item";

describe("ChecklistItem", () => {
  it("routes the failure deep-link into the Help center (§13)", async () => {
    // A raw href would resolve relative to the current path and 404; the Fix link must
    // route the docHref ("troubleshooting#tunarr") into /help as { page, section }.
    render(
      <RouterHarness
        content={
          <ChecklistItem
            name="Tunarr"
            status="fail"
            hint="Could not reach Tunarr"
            docHref="troubleshooting#tunarr"
          />
        }
      />,
    );
    // The router resolves the route asynchronously, so wait for the content to mount.
    expect(await screen.findByText("Could not reach Tunarr")).toBeInTheDocument();
    const href = screen.getByRole("link", { name: /fix/i }).getAttribute("href") ?? "";
    expect(href).toContain("/help");
    expect(href).toContain("page=troubleshooting");
    expect(href).toContain("section=tunarr");
  });

  it("hides the hint on pass and exposes an accessible status", async () => {
    render(
      <RouterHarness
        content={<ChecklistItem name="TMDB" status="pass" hint="unused" docHref="troubleshooting#tmdb" />}
      />,
    );
    expect(await screen.findByRole("img", { name: "Passed" })).toBeInTheDocument();
    expect(screen.queryByText("unused")).not.toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});
