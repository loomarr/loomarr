import type { LibraryItemFileDTO } from "@loomarr/api/models/libraryItemFileDTO";
import { getGetLibraryItemMockHandler, getMeMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppToaster } from "@/components/loomarr/feedback/app-toaster";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { LibraryTitlePage } from "./library-title-page";

const mocks = vi.hoisted(() => ({ navigate: vi.fn(), back: vi.fn(), canGoBack: vi.fn(() => false) }));
vi.mock("@tanstack/react-router", () => ({
  // The router's link as an anchor, so its address is what the test reads.
  Link: ({
    to,
    search,
    params,
    ...rest
  }: {
    to: string;
    search?: Record<string, string>;
    params?: { id: string };
  }) => <a {...rest} href={`${to.replace("$id", params?.id ?? "")}?${new URLSearchParams(search)}`} />,
  useNavigate: () => mocks.navigate,
  useRouter: () => ({ history: { canGoBack: mocks.canGoBack, back: mocks.back } }),
}));

const echo = {
  item: {
    id: "90000003",
    kind: "movie" as const,
    name: "Echo",
    year: 1999,
    files: 1,
    ready: true,
    unplayable: 0,
    extraCopies: 0,
  },
  overview: "A story.",
  genres: ["Drama"],
  studios: [],
  countries: [],
  credits: [],
  files: [
    {
      path: "/media/movies/Echo/Echo.mkv",
      size: 1_000_000,
      ready: true,
      subtitles: [],
      state: "plays" as const,
      streams: [],
      chapters: [],
    },
  ],
  seasons: [],
  match: { by: "file" as const },
  detailsFrom: {},
  channels: [],
  nextAirings: [],
};

const renderPage = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <LibraryTitlePage id="90000003" />
    </QueryClientProvider>,
  );
};

// The wiring: the page shows the server's title, and Back returns to the grid as it was left, or
// to the Library when the page was opened directly.
describe("LibraryTitlePage", () => {
  // The toaster's list outlives a test: without this, one test's toast is on the next one's page.
  afterEach(() => {
    toast.dismiss();
  });

  beforeEach(() => {
    mocks.navigate.mockReset();
    mocks.back.mockReset();
    mocks.canGoBack.mockReset().mockReturnValue(false);
    server.use(
      getMeMockHandler(me()),
      http.get("*/v1/channels", () => HttpResponse.json({ channels: [] })),
      http.get("*/v1/household", () => HttpResponse.json({ timeZone: "UTC" })),
    );
  });

  it("makes a playable channel's row a link to its player, with its logo", async () => {
    const channel = { id: "ch-1", name: "Nightfall", number: 12 };
    const off = { id: "ch-2", name: "Daybreak", number: 13 };
    server.use(
      getGetLibraryItemMockHandler({
        ...echo,
        channels: [channel, off],
        nextAirings: [
          {
            channel,
            start: new Date(Date.now() - 60_000).toISOString(),
            end: new Date(Date.now() + 3_600_000).toISOString(),
            forecast: false,
          },
        ],
      }),
      http.get("*/v1/channels", () =>
        HttpResponse.json({
          channels: [
            { ...channel, logo: "/logo.png", inAppPlayable: true },
            { ...off, inAppPlayable: false },
          ],
        }),
      ),
    );
    const { container } = renderPage();

    const link = await screen.findByRole("link", { name: /Nightfall/ });
    expect(link).toHaveAttribute("href", "/watch/ch-1?");
    expect(link).toHaveTextContent("On now");
    expect(container.querySelector('img[src="/logo.png"]')).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Daybreak/ })).not.toBeInTheDocument();
  });

  it.each([
    ["admin", 1],
    ["member", 0],
  ] as const)("offers an %s %i next step for a title on no channel", async (role, steps) => {
    const meRead = vi.fn();
    server.use(
      getMeMockHandler(() => {
        meRead();
        return me({ role });
      }),
      getGetLibraryItemMockHandler(echo),
    );
    renderPage();

    expect(await screen.findByRole("heading", { name: "Not on a channel yet" })).toBeInTheDocument();
    await waitFor(() => expect(meRead).toHaveBeenCalled());
    await waitFor(() =>
      expect(screen.queryAllByRole("link", { name: "Add it to a collection" })).toHaveLength(steps),
    );
  });

  it("shows the title the server describes", async () => {
    server.use(getGetLibraryItemMockHandler(echo));
    renderPage();

    expect(await screen.findByRole("heading", { name: "Echo" })).toBeInTheDocument();
    expect(screen.getByText("A story.")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Fix" })).toBeInTheDocument();
  });

  const homeVideo = {
    ...echo,
    item: { ...echo.item, name: "Lake day", year: 2019, homeVideo: true },
    overview: undefined,
    genres: ["Home videos"],
    match: { by: "none" as const },
  };

  it("offers Wrong title? but not Update details on a home video, and says what it is", async () => {
    server.use(getGetLibraryItemMockHandler(homeVideo));
    renderPage();

    await userEvent.click(await screen.findByRole("button", { name: "Fix" }));
    expect(await screen.findByRole("menuitem", { name: "Wrong title?" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Check files again" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Update details" })).not.toBeInTheDocument();
    expect(screen.getByText(/A home video you kept as it is/)).toBeInTheDocument();
    expect(screen.getByText("Home videos")).toBeInTheDocument();
  });

  it("gives a member a home video with no fixes and no word about Loomarr's lookups", async () => {
    server.use(getMeMockHandler(me({ role: "member" })), getGetLibraryItemMockHandler(homeVideo));
    renderPage();

    expect(await screen.findByText("A home video from your household's library.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Fix" })).not.toBeInTheDocument();
  });

  it("links the cast and crew the library knows to their titles", async () => {
    server.use(
      getGetLibraryItemMockHandler({
        ...echo,
        credits: [
          { name: "Imelda Varge", role: "actor" as const, personId: "tmdb:90000981" },
          { name: "Tobin Ashcroft", role: "actor" as const, personId: "" },
          { name: "Lorna Pettigrew", role: "director" as const, personId: "tmdb:90000982" },
        ],
      }),
    );
    renderPage();

    expect(await screen.findByRole("link", { name: "Imelda Varge" })).toHaveAttribute(
      "href",
      "/library?person=tmdb%3A90000981",
    );
    expect(screen.getByRole("link", { name: "Lorna Pettigrew" })).toBeInTheDocument();
    expect(screen.getByText("Tobin Ashcroft")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Tobin Ashcroft" })).not.toBeInTheDocument();
  });

  it("offers the three fixes in one menu", async () => {
    const checked = vi.fn();
    server.use(
      getGetLibraryItemMockHandler(echo),
      http.post("*/v1/library/items/:id/check", () => {
        checked();
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPage();

    await userEvent.click(await screen.findByRole("button", { name: "Fix" }));
    for (const name of ["Update details", "Check files again", "Wrong title?"]) {
      expect(await screen.findByRole("menuitem", { name })).toBeInTheDocument();
    }
    await userEvent.click(screen.getByRole("menuitem", { name: "Check files again" }));
    expect(await screen.findByText("Checked just now")).toBeInTheDocument();
    expect(checked).toHaveBeenCalled();
  });

  it("lists each channel's next airing, with the episode and a plain forecast", async () => {
    const channel = { id: "90000971", name: "Quiet Hours", number: 21 };
    const later = { id: "90000972", name: "Late Show", number: 7 };
    server.use(
      getGetLibraryItemMockHandler({
        ...echo,
        channels: [channel, later],
        nextAirings: [
          {
            channel,
            start: new Date(Date.now() - 60_000).toISOString(),
            end: new Date(Date.now() + 3_600_000).toISOString(),
            forecast: true,
            episode: { id: "90000011", season: 1, number: 3, name: "Dead Line" },
          },
        ],
      }),
    );
    renderPage();

    const region = await screen.findByRole("region", { name: "On these channels" });
    expect(region).toHaveTextContent("Quiet Hours");
    expect(region).toHaveTextContent("On now");
    expect(region).toHaveTextContent("Season 1, episode 3 · Dead Line");
    expect(region).toHaveTextContent("Expected time, it can still change");
    expect(region).toHaveTextContent("Late Show");
    expect(region).toHaveTextContent("Nothing scheduled soon");
  });

  it("drops an airing that has ended", async () => {
    const channel = { id: "90000971", name: "Quiet Hours", number: 21 };
    server.use(
      getGetLibraryItemMockHandler({
        ...echo,
        channels: [channel],
        nextAirings: [
          { channel, start: "2020-01-01T00:00:00Z", end: "2020-01-01T01:00:00Z", forecast: false },
        ],
      }),
    );
    renderPage();

    const region = await screen.findByRole("region", { name: "On these channels" });
    expect(region).not.toHaveTextContent("On now");
    expect(region).toHaveTextContent("Nothing scheduled soon");
  });

  it("follows the clock: an airing turns to On now when it starts", async () => {
    const channel = { id: "90000971", name: "Quiet Hours", number: 21 };
    const airing = {
      channel,
      start: new Date(Date.now() + 700).toISOString(),
      end: new Date(Date.now() + 3_600_000).toISOString(),
      forecast: false,
    };
    server.use(getGetLibraryItemMockHandler({ ...echo, channels: [channel], nextAirings: [airing] }));
    renderPage();

    const region = await screen.findByRole("region", { name: "On these channels" });
    expect(region).toHaveTextContent("Next ");
    await waitFor(() => expect(region).toHaveTextContent("On now"), { timeout: 4000 });
  });

  it("says when the title is on no channel", async () => {
    server.use(getGetLibraryItemMockHandler(echo));
    renderPage();

    expect(await screen.findByRole("heading", { name: "Not on a channel yet" })).toBeInTheDocument();
  });

  // #4476: a member reads a title's page with nothing on it that calls an admin route.
  it("gives a member the title with no fixes", async () => {
    const playing = {
      picture: { width: 1920, height: 1080, hdr: false },
      audioChannels: 2,
      subtitleLanguages: ["English"],
    };
    server.use(
      getMeMockHandler(me({ role: "member" })),
      getGetLibraryItemMockHandler({ ...echo, files: undefined, match: undefined, playing }),
    );
    renderPage();

    expect(await screen.findByRole("heading", { name: "Echo" })).toBeInTheDocument();
    expect(screen.getByText("A story.")).toBeInTheDocument();
    expect(screen.getByText("1080p HD · Stereo · English subtitles")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Fix" })).not.toBeInTheDocument();
    expect(screen.queryByText("Technical details")).not.toBeInTheDocument();
  });

  it("goes back to the grid it came from", async () => {
    mocks.canGoBack.mockReturnValue(true);
    server.use(getGetLibraryItemMockHandler(echo));
    renderPage();

    await userEvent.click(await screen.findByRole("button", { name: "Library" }));
    expect(mocks.back).toHaveBeenCalled();
    expect(mocks.navigate).not.toHaveBeenCalled();
  });

  it("goes to the Library when opened directly", async () => {
    server.use(getGetLibraryItemMockHandler(echo));
    renderPage();

    await userEvent.click(await screen.findByRole("button", { name: "Library" }));
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library" });
  });

  const failWith = (status: number) =>
    server.use(
      http.get("*/v1/library/items/:id", () =>
        HttpResponse.json(
          { title: "Failed", status },
          { status, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );

  it("tells a member a title they can't see isn't in the library, with no retry", async () => {
    server.use(getMeMockHandler(me({ role: "member" })));
    failWith(404);
    renderPage();

    expect(
      await screen.findByRole("heading", { name: "This title isn't in the library yet" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /try again/i })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Back to Library" }));
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library" });
  });

  it("keeps the retry for any other failure", async () => {
    failWith(500);
    renderPage();

    expect(await screen.findByRole("button", { name: /try again/i })).toBeInTheDocument();
  });

  // #4476 D3: the removal can be undone from its toast.
  const removeCopy = async (restore: () => Response, press: "once" | "double" = "once") => {
    const restored = vi.fn();
    const copy: LibraryItemFileDTO = {
      path: "/media/movies/Echo/Echo DVD.mkv",
      size: 500_000,
      ready: true,
      subtitles: [],
      picture: { width: 720, height: 480, hdr: false },
      state: "extra" as const,
      streams: [],
      chapters: [],
    };
    // jsdom has no pointer capture, which the toast asks for when its Undo is pressed.
    Element.prototype.setPointerCapture ??= () => {};
    server.use(
      getGetLibraryItemMockHandler({ ...echo, files: [...echo.files, copy] }),
      http.post("*/v1/library/items/:id/remove-copy", () => HttpResponse.json({ undo: "token-1" })),
      http.post("*/v1/library/items/:id/restore-copy", async ({ request }) => {
        restored(await request.json());
        return restore();
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <LibraryTitlePage id="90000003" />
        <AppToaster />
      </QueryClientProvider>,
    );
    await userEvent.click(await screen.findByRole("button", { name: "Remove the SD copy" }));
    expect(await screen.findByText("Moved Echo DVD.mkv to the trash")).toBeInTheDocument();
    const undo = screen.getByRole("button", { name: "Undo" });
    await (press === "double" ? userEvent.dblClick(undo) : userEvent.click(undo));
    return restored;
  };

  it("undoes removing an extra copy with the token the removal answered with", async () => {
    const restored = await removeCopy(() => new HttpResponse(null, { status: 204 }));
    expect(await screen.findByText("Put Echo DVD.mkv back")).toBeInTheDocument();
    expect(restored).toHaveBeenCalledWith({ undo: "token-1" });
  });

  // A refusal is permanent and says why in the server's words (restore-copy's problems).
  it.each([
    [400, "Can't undo this any more", "If you still want the file, it's in the .loomarr-trash folder."],
    [409, "Something is already there", "Move it away, then put the copy back by hand."],
    [410, "The file isn't in the trash", "It may have been emptied or moved."],
  ])("shows the server's words when undo answers %i", async (status, title, detail) => {
    await removeCopy(() => HttpResponse.json({ status, title, detail }, { status }));
    expect(await screen.findByText(title)).toBeInTheDocument();
    expect(screen.getByText(detail)).toBeInTheDocument();
    // Nothing to try again: the removal's own Undo is gone with its toast.
    await waitFor(() => expect(screen.queryByRole("button", { name: "Undo" })).not.toBeInTheDocument());
  });

  it("sends one restore for a double press on Undo", async () => {
    const restored = await removeCopy(() => new HttpResponse(null, { status: 204 }), "double");
    expect(await screen.findByText("Put Echo DVD.mkv back")).toBeInTheDocument();
    expect(restored).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["the server fails", () => HttpResponse.json({ status: 500, title: "boom" }, { status: 500 })],
    ["the network drops", () => HttpResponse.error()],
  ])("offers Undo again with the same token when %s", async (_what, failure) => {
    const answers = [failure, () => new HttpResponse(null, { status: 204 })];
    const restored = await removeCopy(() => (answers.shift() ?? failure)());
    expect(await screen.findByText("Couldn't put Echo DVD.mkv back")).toBeInTheDocument();
    expect(screen.getByText("Try again in a moment.")).toBeInTheDocument();
    // The first toast's button leaves with it; the one that stays is the retry.
    await waitFor(() => expect(screen.getAllByRole("button", { name: "Undo" })).toHaveLength(1));
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(await screen.findByText("Put Echo DVD.mkv back")).toBeInTheDocument();
    expect(restored.mock.calls).toEqual([[{ undo: "token-1" }], [{ undo: "token-1" }]]);
  });

  it("reads an open episode's file again after the title is read again", async () => {
    const episodeRead = vi.fn();
    const series = {
      ...echo,
      item: { ...echo.item, kind: "series" as const },
      files: [],
      seasons: [{ number: 1, episodes: [{ id: "90000004", name: "Pilot", number: 1, ready: true }] }],
    };
    server.use(
      http.get("*/v1/library/items/:id", ({ params }) => {
        if (params.id !== "90000004") return HttpResponse.json(series);
        episodeRead();
        return HttpResponse.json({ ...echo, seasons: [] });
      }),
      http.post("*/v1/library/items/:id/check", () => new HttpResponse(null, { status: 204 })),
    );
    // A cache that would otherwise hand the same answer back.
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 60_000 } } });
    render(
      <QueryClientProvider client={client}>
        <LibraryTitlePage id="90000003" />
      </QueryClientProvider>,
    );
    await userEvent.click(await screen.findByRole("button", { name: /1\. Pilot/ }));
    await waitFor(() => expect(episodeRead).toHaveBeenCalledTimes(1));
    await userEvent.click(screen.getByRole("button", { name: "Fix" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Check files again" }));
    await waitFor(() => expect(episodeRead).toHaveBeenCalledTimes(2));
  });
});
