import { screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ADMIN, approved, MEMBER, renderAt, stub, title } from "@/test/requests-harness";

afterEach(() => vi.restoreAllMocks());

const gaveUp = [{ mediaType: "movie", name: "Gave Up", tmdbId: 90000007, inLibrary: false }];

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
        approved("j-d", "live", [
          { mediaType: "movie", name: "Tin Sheriff 2", tmdbId: 90000008, inLibrary: false },
        ]),
      ],
      titles: { available: [title("Tin Sheriff 2", "available", { tmdbId: 90000008 })] },
    });
    renderAt("/requests/j-d");

    expect(await screen.findByText("All titles are on the channel.")).toBeInTheDocument();
    expect(screen.queryByText("Tin Sheriff 2")).not.toBeInTheDocument();
  });

  it("keeps an outstanding title listed next to landed ones being hidden", async () => {
    stub({
      me: MEMBER,
      journeys: [
        approved("j-d", "building", [
          { mediaType: "movie", name: "Tin Sheriff 2", tmdbId: 90000008, inLibrary: false },
          { mediaType: "movie", name: "Still Coming", tmdbId: 90000009, inLibrary: false },
        ]),
      ],
      titles: {
        available: [title("Tin Sheriff 2", "available", { tmdbId: 90000008 })],
        downloading: [title("Still Coming", "downloading", { tmdbId: 90000009 })],
      },
    });
    renderAt("/requests/j-d");

    expect(await screen.findByText("Still Coming")).toBeInTheDocument();
    expect(screen.queryByText("Tin Sheriff 2")).not.toBeInTheDocument();
  });

  it("renders a title repeated in the lineup without a duplicate-key error", async () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    const dup = { mediaType: "movie", name: "Twice", tmdbId: 90181812 };
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
        unavailable: [title("Gave Up", "unavailable", { tmdbId: 90000007, lastError: "deadline exceeded" })],
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
      titles: { unavailable: [title("Gave Up", "unavailable", { tmdbId: 90000007 })] },
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

describe("RequestDetail while generating", () => {
  const generating = (progress?: unknown) => ({
    version: 1,
    jobId: "j-g",
    milestone: "generating",
    intent: { description: "90s action" },
    attempts: [],
    actions: [],
    createdAt: "2026-09-25T12:00:00Z",
    updatedAt: "2026-09-25T12:00:00Z",
    ...(progress ? { progress } : {}),
  });

  it("shows the live stage and each title as it is chosen", async () => {
    stub({
      me: MEMBER,
      journeys: [
        generating({
          stage: "choosing",
          terms: ["quiet engines"],
          picks: [
            {
              key: "movie:tmdb:90000100",
              mediaType: "movie",
              name: "Quiet Engines",
              year: 1994,
              inLibrary: false,
            },
          ],
          target: 8,
          startedAt: "2026-09-25T12:00:00Z",
        }),
      ],
    });
    renderAt("/requests/j-g");

    expect(await screen.findByText("Choosing titles…")).toBeInTheDocument();
    expect(screen.getByRole("list", { name: "Titles chosen so far" })).toHaveTextContent("Quiet Engines");
    expect(screen.getByText(/1 of about 8 picked · \d+ s/)).toBeInTheDocument();
  });

  it("falls back to the plain waiting line before the first snapshot", async () => {
    stub({ me: MEMBER, journeys: [generating()] });
    renderAt("/requests/j-g");
    expect(await screen.findByText("Understanding your channel…")).toBeInTheDocument();
  });
});
