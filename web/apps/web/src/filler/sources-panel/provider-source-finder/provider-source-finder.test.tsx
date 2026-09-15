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
import { server } from "@/test/msw/server";
import { ProviderSourceFinder } from "./provider-source-finder";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

const suggestion = (id: string, title: string, alreadyAdded = false) => ({
  provider: "archive" as const,
  targetType: "collection" as const,
  canonicalId: id,
  canonicalUrl: `https://archive.org/details/${id}`,
  title,
  itemCount: 120,
  alreadyAdded,
});

const youtubeSuggestion = (id: string, title: string, targetType: "channel" | "playlist" = "channel") => ({
  provider: "youtube" as const,
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
    { title: "First station break", url: "https://archive.org/details/station_break_one", durationMs: 31500 },
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
        return { suggestions: [suggestion("classic_tv", "Classic TV")] };
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

    render(<ProviderSourceFinder kind="archive" enabled />, { wrapper });
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

    await waitFor(() => expect(calls).toEqual([{ kind: "archive", uri: "classic_tv", label: "Classic TV" }]));
  });

  it("supports the keyboard and prevents adding a duplicate", async () => {
    server.use(
      getSuggestFillerSourcesMockHandler({
        suggestions: [
          suggestion("first", "First collection"),
          suggestion("already_here", "Already here", true),
        ],
      }),
      getResolveFillerSourceMockHandler({
        ...suggestion("already_here", "Already here", true),
      }),
    );
    render(<ProviderSourceFinder kind="archive" enabled />, { wrapper });
    const input = screen.getByRole("combobox", { name: "Find an Archive.org collection" });
    await userEvent.type(input, "already");
    await screen.findByRole("option", { name: /first collection/i });
    await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");

    expect(screen.getByText("Already added")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add collection" })).toBeDisabled();
  });

  it("sends a pasted Archive URL straight to exact resolution", async () => {
    let searches = 0;
    let resolvedInput = "";
    server.use(
      getSuggestFillerSourcesMockHandler(() => {
        searches++;
        return { suggestions: [] };
      }),
      getResolveFillerSourceMockHandler(async ({ request }) => {
        const body = (await request.json()) as { input: string };
        resolvedInput = body.input;
        return suggestion("classic_tv", "Classic TV");
      }),
    );
    render(<ProviderSourceFinder kind="archive" enabled />, { wrapper });
    await userEvent.type(
      screen.getByRole("combobox", { name: "Find an Archive.org collection" }),
      "https://archive.org/details/classic_tv",
    );

    await screen.findByRole("button", { name: "Add collection" });
    expect(resolvedInput).toBe("https://archive.org/details/classic_tv");
    expect(searches).toBe(0);
  });

  it("finds a YouTube channel and registers its verified URL", async () => {
    const calls: unknown[] = [];
    server.use(
      getSuggestFillerSourcesMockHandler(({ request }) => {
        expect(new URL(request.url).pathname).toContain("/providers/youtube/suggestions");
        return { suggestions: [youtubeSuggestion("UC-retro", "Retro Reels")] };
      }),
      getResolveFillerSourceMockHandler({
        ...youtubeSuggestion("UC-retro", "Retro Reels"),
        previewItems: [
          {
            title: "Retro ad one",
            url: "https://www.youtube.com/watch?v=video-one",
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

    render(<ProviderSourceFinder kind="youtube" enabled />, { wrapper });
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

  it("sends a pasted YouTube URL straight to exact resolution", async () => {
    let searches = 0;
    let resolvedInput = "";
    server.use(
      getSuggestFillerSourcesMockHandler(() => {
        searches++;
        return { suggestions: [] };
      }),
      getResolveFillerSourceMockHandler(async ({ request }) => {
        const body = (await request.json()) as { input: string };
        resolvedInput = body.input;
        return youtubeSuggestion("PL123", "Favorite breaks", "playlist");
      }),
    );

    render(<ProviderSourceFinder kind="youtube" enabled />, { wrapper });
    await userEvent.type(
      screen.getByRole("combobox", { name: "Find a YouTube channel or playlist" }),
      "https://www.youtube.com/playlist?list=PL123",
    );

    await screen.findByRole("button", { name: "Add source" });
    expect(resolvedInput).toBe("https://www.youtube.com/playlist?list=PL123");
    expect(searches).toBe(0);
    expect(screen.getByText("Playlist")).toBeInTheDocument();
  });

  it("recognizes a YouTube handle without treating it as search text", async () => {
    let searches = 0;
    let resolvedInput = "";
    server.use(
      getSuggestFillerSourcesMockHandler(() => {
        searches++;
        return { suggestions: [] };
      }),
      getResolveFillerSourceMockHandler(async ({ request }) => {
        const body = (await request.json()) as { input: string };
        resolvedInput = body.input;
        return youtubeSuggestion("UC-retro", "Retro Reels");
      }),
    );

    render(<ProviderSourceFinder kind="youtube" enabled />, { wrapper });
    await userEvent.type(
      screen.getByRole("combobox", { name: "Find a YouTube channel or playlist" }),
      "@retroads",
    );

    await screen.findByRole("button", { name: "Add source" });
    expect(resolvedInput).toBe("@retroads");
    expect(searches).toBe(0);
  });

  it("does not search while the provider is paused", async () => {
    let calls = 0;
    server.use(
      getSuggestFillerSourcesMockHandler(() => {
        calls++;
        return { suggestions: [] };
      }),
    );
    render(<ProviderSourceFinder kind="archive" enabled={false} />, { wrapper });
    expect(screen.getByRole("combobox", { name: "Find an Archive.org collection" })).toBeDisabled();
    await new Promise((resolve) => setTimeout(resolve, 350));
    expect(calls).toBe(0);
  });

  it("waits for a useful YouTube query before starting search", async () => {
    let calls = 0;
    server.use(
      getSuggestFillerSourcesMockHandler(() => {
        calls++;
        return { suggestions: [] };
      }),
    );
    render(<ProviderSourceFinder kind="youtube" enabled />, { wrapper });
    await userEvent.type(screen.getByRole("combobox", { name: "Find a YouTube channel or playlist" }), "tv");
    await new Promise((resolve) => setTimeout(resolve, 700));
    expect(calls).toBe(0);
  });

  it("keeps the search text when there are no matches", async () => {
    server.use(getSuggestFillerSourcesMockHandler({ suggestions: [] }));
    render(<ProviderSourceFinder kind="archive" enabled />, { wrapper });
    const emptyInput = screen.getByRole("combobox", { name: "Find an Archive.org collection" });
    await userEvent.type(emptyInput, "no matches");
    await screen.findByText(/no collections found/i);
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
    render(<ProviderSourceFinder kind="archive" enabled />, { wrapper });
    const failedInput = screen.getByRole("combobox", { name: "Find an Archive.org collection" });
    await userEvent.type(failedInput, "classic");
    await screen.findByText(/not responding right now/i);
    expect(failedInput).toHaveValue("classic");
  });
});
