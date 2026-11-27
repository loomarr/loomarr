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
      extra: false,
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
      http.get("*/v1/household", () => HttpResponse.json({ timeZone: "UTC" })),
    );
  });

  it("shows the title the server describes", async () => {
    server.use(getGetLibraryItemMockHandler(echo));
    renderPage();

    expect(await screen.findByRole("heading", { name: "Echo" })).toBeInTheDocument();
    expect(screen.getByText("A story.")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Fix" })).toBeInTheDocument();
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
  const removeCopy = async (restore: () => Response) => {
    const restored = vi.fn();
    const copy: LibraryItemFileDTO = {
      path: "/media/movies/Echo/Echo DVD.mkv",
      size: 500_000,
      ready: true,
      subtitles: [],
      extra: true,
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
    await userEvent.click(await screen.findByRole("button", { name: "Remove Echo DVD.mkv" }));
    expect(await screen.findByText("Moved Echo DVD.mkv to the trash")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    return restored;
  };

  it("undoes removing an extra copy with the token the removal answered with", async () => {
    const restored = await removeCopy(() => new HttpResponse(null, { status: 204 }));
    expect(await screen.findByText("Put Echo DVD.mkv back")).toBeInTheDocument();
    expect(restored).toHaveBeenCalledWith({ undo: "token-1" });
  });

  it.each([410, 409])("says the file is in the trash when undo answers %i", async (status) => {
    await removeCopy(() => HttpResponse.json({ status, title: "no" }, { status }));
    expect(
      await screen.findByText("Couldn't undo. Echo DVD.mkv is in Loomarr's trash for 30 days."),
    ).toBeInTheDocument();
  });
});
