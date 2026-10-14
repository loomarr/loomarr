import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";
import { getChannelGuideMockHandler } from "@loomarr/api/msw";
import { ideaReason } from "@loomarr/core/requests";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { MEMBER, renderAt, stub } from "@/test/requests-harness";

const DAY_MS = 24 * 60 * 60 * 1000;

const idea = (over: Partial<ChannelIdeaDTO> & Pick<ChannelIdeaDTO, "id" | "name">): ChannelIdeaDTO => ({
  facet: "genre",
  value: "Comedy",
  pitch: "Every comedy title in your library that no channel plays yet, on one channel.",
  reason: { kind: "unaired", count: 14 },
  keys: ["m1", "m2", "m3", "m4"],
  titles: [],
  movies: 14,
  series: 0,
  inLibrary: 14,
  toDownload: 0,
  requested: false,
  ...over,
});

describe("ideaReason — the client words the server's typed reason (#1720)", () => {
  it("counts a genre's unaired titles by what they are", () => {
    expect(ideaReason(idea({ id: "genre:comedy", name: "Comedy Movies" }), false, 0)).toBe(
      "14 comedy movies, none on a channel yet",
    );
  });

  it("counts the library instead when nothing is on the air", () => {
    const shows = idea({
      id: "genre:comedy",
      name: "Comedy Shows",
      movies: 0,
      series: 9,
      reason: { kind: "unaired", count: 9 },
    });
    expect(ideaReason(shows, true, 0)).toBe("9 comedy series in your library");
  });

  it("names a decade the way the idea's name does", () => {
    const mix = idea({ id: "decade:1990", name: "90s Channel", facet: "decade", value: "1990", series: 3 });
    expect(ideaReason(mix, false, 0)).toBe("14 titles from the 90s, none on a channel yet");
  });

  it("says how far off a holiday is, in weeks as the mock does", () => {
    const now = Date.UTC(2026, 8, 24);
    const holiday = (startsAtMs: number) =>
      idea({
        id: "holiday:halloween",
        name: "Halloween Channel",
        facet: "holiday",
        value: "halloween",
        reason: { kind: "holiday", count: 31, holidayLabel: "Halloween", startsAtMs },
      });
    expect(ideaReason(holiday(now + 37 * DAY_MS), false, now)).toBe("Halloween is five weeks away");
    expect(ideaReason(holiday(now + 3 * DAY_MS), false, now)).toBe("Halloween is 3 days away");
    expect(ideaReason(holiday(now - DAY_MS), false, now)).toBe("Halloween is on now");
  });
});

// A member's ideas through the real route, against a server that remembers requests and hides.
const serve = (ideas: ChannelIdeaDTO[]) => {
  const hidden = new Set<string>();
  const requested = new Set<string>();
  const calls: string[] = [];
  server.use(
    http.get("*/v1/discovery/ideas", () =>
      HttpResponse.json({
        ideas: ideas.filter((i) => !hidden.has(i.id)).map((i) => ({ ...i, requested: requested.has(i.id) })),
      }),
    ),
    http.post("*/v1/discovery/ideas/:ideaId/request", ({ params }) => {
      calls.push(`request ${params.ideaId}`);
      requested.add(String(params.ideaId));
      return HttpResponse.json({ jobId: "j1" }, { status: 202 });
    }),
    http.put("*/v1/me/hidden-ideas/:ideaId", ({ params }) => {
      calls.push(`hide ${params.ideaId}`);
      hidden.add(String(params.ideaId));
      return new HttpResponse(null, { status: 204 });
    }),
    http.delete("*/v1/me/hidden-ideas/:ideaId", ({ params }) => {
      calls.push(`unhide ${params.ideaId}`);
      hidden.delete(String(params.ideaId));
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return calls;
};

const onAir = () =>
  server.use(
    getChannelGuideMockHandler({
      channels: [{ airings: [], channelId: "c1", name: "Test", number: 7, pendingCount: 0, status: "live" }],
      fromMs: 0,
      toMs: 0,
    }),
  );

const FOUR = [
  idea({ id: "genre:comedy", name: "Comedy Movies" }),
  idea({ id: "genre:drama", name: "Drama Movies", value: "Drama" }),
  idea({ id: "genre:western", name: "Western Movies", value: "Western", toDownload: 2, inLibrary: 12 }),
  idea({ id: "decade:1980", name: "80s Movies", facet: "decade", value: "1980" }),
];

const card = (name: string) => screen.getByRole("article", { name });

afterEach(() => vi.restoreAllMocks());

describe("Channel ideas on a member's Home (#1659)", () => {
  it("requests an idea, and the card then waits for an admin", async () => {
    stub({ me: MEMBER });
    onAir();
    const calls = serve(FOUR);
    renderAt("/dashboard");

    expect(await screen.findByText("Picked from your library")).toBeInTheDocument();
    expect(screen.getAllByRole("article")).toHaveLength(3);
    expect(card("Western Movies")).toHaveTextContent("14 movies · 12 in your library, 2 to download");

    await userEvent.click(within(card("Comedy Movies")).getByRole("button", { name: "Request channel" }));
    expect(await within(card("Comedy Movies")).findByText("Waiting for an admin")).toBeInTheDocument();
    expect(within(card("Comedy Movies")).queryByRole("button")).not.toBeInTheDocument();
    expect(calls).toEqual(["request genre:comedy"]);
  });

  it("hides an idea with an undo, and pages through the rest", async () => {
    stub({ me: MEMBER });
    onAir();
    const calls = serve(FOUR);
    renderAt("/dashboard");

    await userEvent.click(await screen.findByRole("button", { name: "Not for me: Drama Movies" }));
    expect(await screen.findByText("Hid Drama Movies")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.queryByRole("article", { name: "Drama Movies" })).not.toBeInTheDocument(),
    );
    // Three left, so there's nothing to page through.
    expect(screen.queryByRole("button", { name: "Different ideas" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(await screen.findByRole("button", { name: "Different ideas" })).toBeInTheDocument();
    expect(screen.queryByText("Hid Drama Movies")).not.toBeInTheDocument();
    expect(calls).toEqual(["hide genre:drama", "unhide genre:drama"]);

    await userEvent.click(screen.getByRole("button", { name: "Different ideas" }));
    expect(screen.getAllByRole("article").map((a) => a.getAttribute("aria-labelledby"))).toEqual([
      "idea-decade:1980",
      "idea-genre:comedy",
      "idea-genre:drama",
    ]);
  });

  // "Edit first" and "Describe your own channel" (#1817, #1872) both open the Guide's manual
  // builder — the shipped grid used to name these as waiting on the mock.
  it("links Edit first and Describe your own channel into the Guide's manual builder", async () => {
    stub({ me: MEMBER });
    onAir();
    serve(FOUR);
    renderAt("/dashboard");

    await screen.findByText("Picked from your library");
    const editFirst = await within(card("Comedy Movies")).findByRole("link", { name: "Edit first" });
    expect(editFirst).toHaveAttribute("href", "/guide?manual=%221%22&ideaId=genre%3Acomedy");

    expect(screen.getByRole("link", { name: /describe your own channel/i })).toHaveAttribute(
      "href",
      "/guide?manual=%221%22",
    );
  });

  it("offers ideas on an empty Home, and says when there are none left", async () => {
    stub({ me: MEMBER });
    serve([]);
    renderAt("/dashboard");

    expect(
      await screen.findByText("Nothing’s on yet. Start with one of these, built from your library."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("That’s every idea for now. New ones appear as your library grows."),
    ).toBeInTheDocument();
  });
});
