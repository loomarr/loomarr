import type { ClipDTO } from "@loomarr/api";
import { getListFillerMockHandler, getListTaxonomyMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { FillerCatalog } from "./filler-catalog";

const clip: ClipDTO = {
  hash: "catalog-hash",
  name: "Local soda commercial",
  kind: "commercial",
  era: 1990,
  audience: "general",
  category: "drinks",
  durationMs: 30_000,
  source: "folder",
  playCount: 0,
  playsCounted: true,
  aiTagged: false,
  tagged: true,
  suggestedEra: 0,
};

const Wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
  >
    {children}
  </QueryClientProvider>
);

describe("FillerCatalog", () => {
  it("opens the exact clip panel before optional editing", async () => {
    const detailedClip: ClipDTO = {
      ...clip,
      media: {
        width: 1920,
        height: 1080,
        frameRate: "24000/1001",
        container: "mp4",
        videoCodec: "h264",
        audioCodec: "aac",
        audioChannels: 2,
        audioRateHz: 48000,
        bytes: 9361105,
      },
      enrichment: {
        state: "details_limited",
        facts: [{ axis: "kind", evidence: "item_metadata" }],
      },
    };
    server.use(
      getListFillerMockHandler({ clips: [detailedClip], total: 1 }),
      getListTaxonomyMockHandler({
        taxa: [],
        totalClips: 1,
        taggedClips: 1,
        unclassifiedClips: 0,
        axisCoverage: [],
      }),
    );

    render(<RouterHarness initialPath="/filler/library" content={<FillerCatalog isAdmin />} />, {
      wrapper: Wrapper,
    });

    expect(await screen.findByText("Local soda commercial")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "View details for Local soda commercial" }));
    const panel = await screen.findByRole("dialog", { name: "Local soda commercial" });
    expect(within(panel).getByText("Details limited")).toBeInTheDocument();
    await userEvent.click(within(panel).getByText("More about this clip"));
    expect(within(panel).getByText("Item details")).toBeInTheDocument();
    expect(within(panel).getByText(/1920×1080.*H\.264.*23\.98 fps/i)).toBeInTheDocument();
    expect(within(panel).getByText(/AAC.*stereo.*48 kHz/i)).toBeInTheDocument();
    expect(within(panel).getByText(/MP4.*8\.9 MB/i)).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Edit details" })).toBeInTheDocument();
    await userEvent.click(within(panel).getByRole("button", { name: "Edit details" }));
    expect(
      await screen.findByRole("region", { name: "Edit details: Local soda commercial" }),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("clears a selected clip when a search removes it from the rendered result set", async () => {
    const otherClip: ClipDTO = { ...clip, hash: "other-catalog-hash", name: "Weather report" };
    server.use(
      getListFillerMockHandler(({ request }) => {
        const q = new URL(request.url).searchParams.get("q");
        return q === "weather" ? { clips: [otherClip], total: 1 } : { clips: [clip], total: 1 };
      }),
    );

    const user = userEvent.setup();
    render(<RouterHarness initialPath="/filler/library" content={<FillerCatalog isAdmin />} />, {
      wrapper: Wrapper,
    });

    await screen.findByText("Local soda commercial");
    await user.click(screen.getByRole("checkbox", { name: "Select Local soda commercial" }));
    expect(await screen.findByText("1 clip selected")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Search"), "weather");
    expect(await screen.findByText("Weather report")).toBeInTheDocument();
    expect(screen.queryByText("Local soda commercial")).not.toBeInTheDocument();

    // Selection is transient intent about the rows in front of the operator. Keeping this bar
    // armed would let its hash-keyed bulk actions target a row hidden by the new server result.
    expect(screen.queryByText("1 clip selected")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove from catalog" })).not.toBeInTheDocument();
  });

  it("keeps a selected clip when switching the view on the same result page", async () => {
    server.use(getListFillerMockHandler({ clips: [clip], total: 1 }));

    const user = userEvent.setup();
    render(<RouterHarness initialPath="/filler/library" content={<FillerCatalog isAdmin />} />, {
      wrapper: Wrapper,
    });

    await screen.findByText("Local soda commercial");
    await user.click(screen.getByRole("checkbox", { name: "Select Local soda commercial" }));
    await user.click(screen.getByRole("radio", { name: "List" }));

    expect(await screen.findByText("1 clip selected")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove from catalog" })).toBeInTheDocument();
  });
  it("List inspection preserves view, filters, page, selection and return focus", async () => {
    server.use(getListFillerMockHandler({ clips: [clip], total: 121 }));
    const user = userEvent.setup();
    render(
      <RouterHarness
        initialPath="/filler/library?view=list&q=soda&page=2"
        content={<FillerCatalog isAdmin />}
      />,
      { wrapper: Wrapper },
    );
    const title = await screen.findByRole("button", { name: "View details for Local soda commercial" });
    await user.click(screen.getByRole("checkbox", { name: "Select Local soda commercial" }));
    await user.click(title);
    const panel = await screen.findByRole("dialog", { name: "Local soda commercial" });
    expect(within(panel).getByText("1990s")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(title).toHaveFocus());
    expect(screen.getByRole("radio", { name: "List" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByLabelText("Search")).toHaveValue("soda");
    expect(screen.getByText("Page 2 of 3")).toBeInTheDocument();
    expect(screen.getByText("1 clip selected")).toBeInTheDocument();
    await user.click(screen.getByRole("radio", { name: "Grid" }));
    expect(screen.getByText("Page 2 of 3")).toBeInTheDocument();
    expect(screen.getByText("1 clip selected")).toBeInTheDocument();
  });

  it("opens a nested recording segment by exact hash rather than the current result page", async () => {
    const recording: ClipDTO = { ...clip, hash: "recording", name: "Source recording", isComposite: true };
    const child: ClipDTO = { ...clip, hash: "child", name: "Nested clip", parentHash: "recording" };
    const queries: string[] = [];
    server.use(
      getListFillerMockHandler(({ request }) => {
        const params = new URL(request.url).searchParams;
        queries.push(params.toString());
        return {
          clips:
            params.get("hashes") === "child" || params.get("parentHash") === "recording"
              ? [child]
              : [recording],
          total: 1,
        };
      }),
    );
    const user = userEvent.setup();
    render(<RouterHarness initialPath="/filler/library?view=list" content={<FillerCatalog isAdmin />} />, {
      wrapper: Wrapper,
    });
    await user.click(await screen.findByRole("button", { name: "Show segments from Source recording" }));
    await user.click(await screen.findByRole("button", { name: "View details for Nested clip" }));
    expect(await screen.findByRole("dialog", { name: "Nested clip" })).toBeInTheDocument();
    expect(queries.some((query) => new URLSearchParams(query).get("hashes") === "child")).toBe(true);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });
});
