import { screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ADMIN, approved, MEMBER, renderAt, stub, title } from "@/test/requests-harness";

afterEach(() => vi.restoreAllMocks());

const gaveUp = [{ mediaType: "movie", name: "Gave Up", tmdbId: 7, inLibrary: false }];

describe("RequestDetail", () => {
  // An approval that has not produced its titles yet must say so, not drop the section.
  it("says titles appear once the channel starts building, instead of an empty section", async () => {
    stub({ me: MEMBER, journeys: [approved("j-d", "building")] });
    renderAt("/requests/j-d");

    expect(await screen.findByRole("heading", { name: "Titles being added" })).toBeInTheDocument();
    expect(screen.getByText("Titles appear once the channel starts building.")).toBeInTheDocument();
  });

  // A landed title is part of the channel: listing it as "being added" contradicts a live channel.
  it("lists only outstanding titles, and says so when everything is on the channel", async () => {
    stub({
      me: MEMBER,
      journeys: [
        approved("j-d", "live", [{ mediaType: "movie", name: "RoboCop 2", tmdbId: 8, inLibrary: false }]),
      ],
      titles: { available: [title("RoboCop 2", "available", { tmdbId: 8 })] },
    });
    renderAt("/requests/j-d");

    expect(await screen.findByText("All titles are on the channel.")).toBeInTheDocument();
    expect(screen.queryByText("RoboCop 2")).not.toBeInTheDocument();
  });

  it("keeps an outstanding title listed next to landed ones being hidden", async () => {
    stub({
      me: MEMBER,
      journeys: [
        approved("j-d", "building", [
          { mediaType: "movie", name: "RoboCop 2", tmdbId: 8, inLibrary: false },
          { mediaType: "movie", name: "Still Coming", tmdbId: 9, inLibrary: false },
        ]),
      ],
      titles: {
        available: [title("RoboCop 2", "available", { tmdbId: 8 })],
        downloading: [title("Still Coming", "downloading", { tmdbId: 9 })],
      },
    });
    renderAt("/requests/j-d");

    expect(await screen.findByText("Still Coming")).toBeInTheDocument();
    expect(screen.queryByText("RoboCop 2")).not.toBeInTheDocument();
  });

  it("renders a title repeated in the lineup without a duplicate-key error", async () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    const dup = { mediaType: "movie", name: "Twice", tmdbId: 181812 };
    stub({
      me: MEMBER,
      journeys: [
        {
          ...approved("j-d", "live"),
          proposal: {
            id: "p",
            status: "approved",
            proposal: { intent: { description: "x" }, lineup: [dup, dup], acquisitions: [] },
          },
        },
      ],
    });
    renderAt("/requests/j-d");

    expect(await screen.findAllByText("Twice")).toHaveLength(2);
    expect(error.mock.calls.flat().join(" ")).not.toMatch(/same key/);
  });

  it("names who approved it", async () => {
    stub({ me: MEMBER, journeys: [approved("j-d", "building")] });
    renderAt("/requests/j-d");
    expect(await screen.findByText(/Approved by Ada/)).toBeInTheDocument();
  });

  it("lets an admin retry a given-up title, with why it was given up", async () => {
    stub({
      me: ADMIN,
      journeys: [approved("j-d", "building", gaveUp)],
      titles: {
        unavailable: [title("Gave Up", "unavailable", { tmdbId: 7, lastError: "deadline exceeded" })],
      },
    });
    renderAt("/requests/j-d");

    expect(await screen.findByText(/deadline exceeded/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Try again/ })).toBeInTheDocument();
  });

  it("offers a member no Try again button — re-enqueueing is admin-only", async () => {
    stub({
      me: MEMBER,
      journeys: [approved("j-d", "building", gaveUp)],
      titles: { unavailable: [title("Gave Up", "unavailable", { tmdbId: 7 })] },
    });
    renderAt("/requests/j-d");

    await screen.findByText("Gave Up");
    expect(screen.queryByRole("button", { name: /Try again/ })).not.toBeInTheDocument();
  });

  it("shows no request content when it cannot be opened", async () => {
    stub({ me: MEMBER });
    renderAt("/requests/nope");
    await waitFor(() => expect(screen.queryByText("Loading…")).not.toBeInTheDocument());
    expect(screen.queryByRole("heading", { name: "Titles being added" })).not.toBeInTheDocument();
  });
});
