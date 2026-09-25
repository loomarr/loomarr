import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { failed, journey, MEMBER, renderAt, stub } from "@/test/requests-harness";

afterEach(() => vi.restoreAllMocks());

// useRequests / useNeedsYouCount feed the tab counts and the nav badge, so they are asserted
// where a viewer sees them: the list is the viewer's own, and the badge counts what needs them.
describe("useRequests", () => {
  it("lists only the viewer's own requests, admins included", async () => {
    const seen = stub({ me: MEMBER, journeys: [journey("j-gen", { milestone: "generating" })] });
    renderAt("/requests/in-progress");
    await screen.findByText("Request j-gen");

    const asked = seen.filter((u) => u.startsWith("/v1/proposal-jobs"));
    expect(asked).not.toHaveLength(0);
    expect(asked.every((u) => u.includes("mine=true"))).toBe(true);
  });
});

describe("useNeedsYouCount", () => {
  it("shows what needs the viewer on the Requests nav entry", async () => {
    stub({ me: MEMBER, journeys: [failed("j-bad"), failed("j-bad2")] });
    renderAt("/requests/in-progress");
    expect(await screen.findByTestId("nav-badge-/requests")).toHaveTextContent("2");
  });

  it("shows nothing when nothing needs the viewer", async () => {
    stub({ me: MEMBER, journeys: [journey("j-gen", { milestone: "generating" })] });
    renderAt("/requests/in-progress");
    await screen.findByText("Request j-gen");
    expect(screen.queryByTestId("nav-badge-/requests")).not.toBeInTheDocument();
  });
});
