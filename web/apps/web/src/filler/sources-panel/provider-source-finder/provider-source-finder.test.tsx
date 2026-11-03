import {
  getAddFillerSourceMockHandler,
  getResolveFillerSourceMockHandler,
  getSuggestFillerSourcesMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { fillerProviders } from "@/test/fixtures/filler";
import { server } from "@/test/msw/server";
import { ProviderSourceFinder } from "./provider-source-finder";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

const providerNamed = (id: string) => {
  const provider = fillerProviders.find((candidate) => candidate.id === id);
  if (!provider) throw new Error(`no ${id} provider in the fixture registry`);
  return provider;
};
const archiveProvider = providerNamed("archive");
const youtubeProvider = providerNamed("youtube");

// A provider the web has never heard of: every word and number on its finder comes from this.
const thirdProvider = {
  ...archiveProvider,
  id: "thirdparty",
  label: "Third Party",
  findLabel: "Find a Third Party feed",
  findPlaceholder: "Look a feed up",
  findAdd: "Add feed",
  findNoMatches: "Nothing here. Paste the feed's address.",
  findMinQuery: 4,
  previewHeading: "Samples from Third Party",
};

const suggestion = (id: string, title: string, alreadyAdded = false) => ({
  provider: "archive",
  targetType: "collection" as const,
  canonicalId: id,
  canonicalUrl: `https://archive.org/details/${id}`,
  title,
  itemCount: 120,
  alreadyAdded,
});

const youtubeSuggestion = (id: string, title: string, targetType: "channel" | "playlist" = "channel") => ({
  provider: "youtube",
  targetType,
  canonicalId: id,
  canonicalUrl:
    targetType === "channel"
      ? `https://www.youtube.com/channel/${id}/videos`
      : `https://www.youtube.com/playlist?list=${id}`,
  title,
  alreadyAdded: false,
});

const archivePreview = {
  ...suggestion("classic_tv", "Classic TV"),
  previewItems: [
    {
      title: "First station break",
      url: "https://archive.org/details/station_break_one",
      embedUrl: "https://archive.org/embed/station_break_one?autoplay=1",
      durationMs: 31500,
    },
    { title: "Second station break", url: "https://archive.org/details/station_break_two" },
  ],
};

describe("ProviderSourceFinder", () => {
  it("searches after typing and registers only after explicit confirmation", async () => {
    const calls: unknown[] = [];
    let resolvedInput = "";
    server.use(
      getSuggestFillerSourcesMockHandler(({ request }) => {
        expect(new URL(request.url).searchParams.get("q")).toBe("classic tv");
        return { exact: false, suggestions: [suggestion("classic_tv", "Classic TV")] };
      }),
      getResolveFillerSourceMockHandler(async ({ request }) => {
        const body = (await request.json()) as { input: string };
        resolvedInput = body.input;
        return archivePreview;
      }),
      getAddFillerSourceMockHandler(async ({ request }) => {
        calls.push(await request.json());
        return { id: "archive:classic_tv", label: "Classic TV", uri: "classic_tv", enabled: true };
      }),
    );

    render(<ProviderSourceFinder provider={archiveProvider} enabled />, { wrapper });
    await userEvent.type(
      screen.getByRole("combobox", { name: "Find an Archive.org collection" }),
      "classic tv",
    );
    await userEvent.click(await screen.findByRole("option", { name: /classic tv/i }));

    expect(calls).toHaveLength(0);
    await screen.findByRole("button", { name: "Add collection" });
    expect(resolvedInput).toBe("https://archive.org/details/classic_tv");
    expect(screen.getByText("120 items")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /open on archive.org/i })).toHaveAttribute(
      "href",
      "https://archive.org/details/classic_tv",
    );
    expect(screen.getByRole("link", { name: "First station break" })).toHaveAttribute(
      "href",
      "https://archive.org/details/station_break_one",
    );
    expect(screen.getByText("Popular videos in this collection")).toBeInTheDocument();
    expect(screen.getByText("32s")).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Preview" })[0]!);
    expect(screen.getByTitle("Preview First station break")).toHaveAttribute(
      "src",
      "https://archive.org/embed/station_break_one?autoplay=1",
    );
    expect(screen.getByTitle("Preview First station break")).toHaveAttribute(
      "allow",
      "autoplay; encrypted-media; picture-in-picture",
    );
    expect(calls).toHaveLength(0);
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await userEvent.click(screen.getByRole("button", { name: "Add collection" }));

    await waitFor(() =>
      expect(calls).toEqual([
        { kind: "archive", uri: "https://archive.org/details/classic_tv", label: "Classic TV" },
      ]),
    );
  });

  it("takes an address the server recognized as the checked source, without a second check", async () => {
    let resolutions = 0;
    server.use(
      getSuggestFillerSourcesMockHandler({ exact: true, suggestions: [archivePreview] }),
      getResolveFillerSourceMockHandler(() => {
        resolutions++;
        return archivePreview;
      }),
    );

    render(<ProviderSourceFinder provider={archiveProvider} enabled />, { wrapper });
    const input = screen.getByRole("combobox", { name: "Find an Archive.org collection" });
    // The page does not know what an address looks like: the server's answer says so.
    await userEvent.type(input, "anything the provider recognizes");

    await screen.findByRole("button", { name: "Add collection" });
    expect(input).toHaveValue("Classic TV");
    expect(screen.getByText("Popular videos in this collection")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Preview" })).toHaveLength(2);
    expect(resolutions).toBe(0);
  });

  it("cancels a checked source without adding it and returns focus to search", async () => {
    const calls: unknown[] = [];
    server.use(
      getSuggestFillerSourcesMockHandler({ exact: true, suggestions: [archivePreview] }),
      getAddFillerSourceMockHandler(async ({ request }) => {
        calls.push(await request.json());
        return { id: "archive:classic_tv", label: "Classic TV", uri: "classic_tv", enabled: true };
      }),
    );

    render(<ProviderSourceFinder provider={archiveProvider} enabled />, { wrapper });
    const input = screen.getByRole("combobox", { name: "Find an Archive.org collection" });
    await userEvent.type(input, "https://archive.org/details/classic_tv");
    await screen.findByRole("button", { name: "Add collection" });

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(input).toHaveValue("");
    expect(input).toHaveFocus();
    expect(screen.queryByText("Popular videos in this collection")).not.toBeInTheDocument();
    expect(calls).toHaveLength(0);
  });

  it("does not replace newer search text with a stale exact-source response", async () => {
    let releaseSearch: (() => void) | undefined;
    server.use(
      getSuggestFillerSourcesMockHandler(async ({ request }) => {
        if (new URL(request.url).searchParams.get("q") === "https://archive.org/details/classic_tv") {
          await new Promise<void>((resolve) => {
            releaseSearch = resolve;
          });
          return { exact: true, suggestions: [archivePreview] };
        }
        return { exact: false, suggestions: [] };
      }),
    );

    render(<ProviderSourceFinder provider={archiveProvider} enabled />, { wrapper });
    const input = screen.getByRole("combobox", { name: "Find an Archive.org collection" });
    await userEvent.type(input, "https://archive.org/details/classic_tv");
    await screen.findByRole("button", { name: "Searching…" });
    await userEvent.clear(input);
    await userEvent.type(input, "new collection");
    releaseSearch?.();

    await waitFor(() => expect(screen.queryByRole("button", { name: "Searching…" })).not.toBeInTheDocument());
    expect(input).toHaveValue("new collection");
    expect(screen.queryByRole("button", { name: "Add collection" })).not.toBeInTheDocument();
  });

  it("supports the keyboard and prevents adding a duplicate", async () => {
    server.use(
      getSuggestFillerSourcesMockHandler({
        exact: false,
        suggestions: [
          suggestion("first", "First collection"),
          suggestion("already_here", "Already here", true),
        ],
      }),
      getResolveFillerSourceMockHandler({
        ...suggestion("already_here", "Already here", true),
      }),
    );
    render(<ProviderSourceFinder provider={archiveProvider} enabled />, { wrapper });
    const input = screen.getByRole("combobox", { name: "Find an Archive.org collection" });
    await userEvent.type(input, "already");
    await screen.findByRole("option", { name: /first collection/i });
    await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");

    expect(screen.getByText("Already added")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add collection" })).toBeDisabled();
  });

  it("finds a YouTube channel and registers its verified URL", async () => {
    const calls: unknown[] = [];
    server.use(
      getSuggestFillerSourcesMockHandler(({ request }) => {
        expect(new URL(request.url).pathname).toContain("/providers/youtube/suggestions");
        return { exact: false, suggestions: [youtubeSuggestion("UC-retro", "Retro Reels")] };
      }),
      getResolveFillerSourceMockHandler({
        ...youtubeSuggestion("UC-retro", "Retro Reels"),
        previewItems: [
          {
            title: "Retro ad one",
            url: "https://www.youtube.com/watch?v=video-one",
            embedUrl: "https://www.youtube-nocookie.com/embed/video-one?autoplay=1&playsinline=1",
            durationMs: 45000,
          },
        ],
      }),
      getAddFillerSourceMockHandler(async ({ request }) => {
        calls.push(await request.json());
        return {
          id: "youtube:UC-retro",
          label: "Retro Reels",
          uri: "https://www.youtube.com/channel/UC-retro/videos",
          enabled: true,
        };
      }),
    );

    render(<ProviderSourceFinder provider={youtubeProvider} enabled />, { wrapper });
    await userEvent.type(
      screen.getByRole("combobox", { name: "Find a YouTube channel or playlist" }),
      "retro commercials",
    );
    expect(screen.getByRole("button", { name: "Search" })).toBeInTheDocument();
    await userEvent.click(await screen.findByRole("option", { name: /retro reels/i }));

    await screen.findByRole("button", { name: "Add source" });
    expect(screen.getByText("Channel")).toBeInTheDocument();
    expect(screen.getByText("A few videos from this source")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Preview" }));
    expect(screen.getByTitle("Preview Retro ad one")).toHaveAttribute(
      "src",
      "https://www.youtube-nocookie.com/embed/video-one?autoplay=1&playsinline=1",
    );
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await userEvent.click(screen.getByRole("button", { name: "Add source" }));
    await waitFor(() =>
      expect(calls).toEqual([
        {
          kind: "youtube",
          uri: "https://www.youtube.com/channel/UC-retro/videos",
          label: "Retro Reels",
        },
      ]),
    );
  });

  it("does not search while the provider is paused", async () => {
    let calls = 0;
    server.use(
      getSuggestFillerSourcesMockHandler(() => {
        calls++;
        return { exact: false, suggestions: [] };
      }),
    );
    render(<ProviderSourceFinder provider={archiveProvider} enabled={false} />, { wrapper });
    expect(screen.getByRole("combobox", { name: "Find an Archive.org collection" })).toBeDisabled();
    await new Promise((resolve) => setTimeout(resolve, 650));
    expect(calls).toBe(0);
  });

  it("waits for the number of characters the provider says a search is worth", async () => {
    let calls = 0;
    server.use(
      getSuggestFillerSourcesMockHandler(() => {
        calls++;
        return { exact: false, suggestions: [] };
      }),
    );
    render(<ProviderSourceFinder provider={youtubeProvider} enabled />, { wrapper });
    await userEvent.type(screen.getByRole("combobox", { name: "Find a YouTube channel or playlist" }), "tv");
    await new Promise((resolve) => setTimeout(resolve, 650));
    expect(calls).toBe(0);
  });

  it("keeps the search text when there are no matches, and says what the provider says to do", async () => {
    server.use(getSuggestFillerSourcesMockHandler({ exact: false, suggestions: [] }));
    render(<ProviderSourceFinder provider={archiveProvider} enabled />, { wrapper });
    const emptyInput = screen.getByRole("combobox", { name: "Find an Archive.org collection" });
    await userEvent.type(emptyInput, "no matches");
    await screen.findByText("No collections found. Paste the exact URL to check it.");
    expect(emptyInput).toHaveValue("no matches");
  });

  it("keeps the search text when Archive.org is unavailable", async () => {
    server.use(
      http.get("*/v1/filler/providers/archive/suggestions", () =>
        HttpResponse.json(
          { title: "Bad Gateway", status: 502, detail: "Archive.org is not responding right now." },
          { status: 502 },
        ),
      ),
    );
    render(<ProviderSourceFinder provider={archiveProvider} enabled />, { wrapper });
    const failedInput = screen.getByRole("combobox", { name: "Find an Archive.org collection" });
    await userEvent.type(failedInput, "classic");
    await screen.findByText(/not responding right now/i);
    expect(failedInput).toHaveValue("classic");
  });

  it("names the provider by the label the registry gave it when a failure carries no detail", async () => {
    server.use(
      http.get("*/v1/filler/providers/archive/suggestions", () => HttpResponse.json({}, { status: 502 })),
    );
    render(<ProviderSourceFinder provider={{ ...archiveProvider, label: "Registry Name" }} enabled />, {
      wrapper,
    });
    await userEvent.type(screen.getByRole("combobox", { name: "Find an Archive.org collection" }), "classic");
    await screen.findByText("Registry Name is not responding right now. Try again in a moment.");
  });

  // A provider added to the server's registry gets a working finder from its description alone.
  it("renders and works for a provider the page has never heard of", async () => {
    const calls: unknown[] = [];
    const searches: string[] = [];
    server.use(
      getSuggestFillerSourcesMockHandler(({ request }) => {
        const url = new URL(request.url);
        searches.push(`${url.pathname}?q=${url.searchParams.get("q")}`);
        return {
          exact: false,
          suggestions: [
            {
              ...suggestion("feed_one", "Feed One"),
              provider: "thirdparty",
              canonicalUrl: "https://third.example/feed_one",
            },
          ],
        };
      }),
      getResolveFillerSourceMockHandler({
        ...suggestion("feed_one", "Feed One"),
        provider: "thirdparty",
        canonicalUrl: "https://third.example/feed_one",
        previewItems: [{ title: "Sample one", url: "https://third.example/one" }],
      }),
      getAddFillerSourceMockHandler(async ({ request }) => {
        calls.push(await request.json());
        return {
          id: "thirdparty:feed_one",
          label: "Feed One",
          uri: "https://third.example/feed_one",
          enabled: true,
        };
      }),
    );

    render(<ProviderSourceFinder provider={thirdProvider} enabled />, { wrapper });
    const input = screen.getByRole("combobox", { name: "Find a Third Party feed" });
    expect(input).toHaveAttribute("placeholder", "Look a feed up");

    await userEvent.type(input, "fee");
    await new Promise((resolve) => setTimeout(resolve, 650));
    expect(searches).toEqual([]);

    await userEvent.type(input, "d one");
    await userEvent.click(await screen.findByRole("option", { name: /feed one/i }));
    const add = await screen.findByRole("button", { name: "Add feed" });
    expect(await screen.findByText("Samples from Third Party")).toBeInTheDocument();
    await userEvent.click(add);

    expect(searches).toEqual(["/v1/filler/providers/thirdparty/suggestions?q=feed one"]);
    await waitFor(() =>
      expect(calls).toEqual([
        { kind: "thirdparty", uri: "https://third.example/feed_one", label: "Feed One" },
      ]),
    );
  });
});
