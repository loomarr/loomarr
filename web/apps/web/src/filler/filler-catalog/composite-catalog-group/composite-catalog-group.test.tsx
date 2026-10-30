import type { ClipDTO } from "@loomarr/api";
import { getListFillerMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { CompositeCatalogGroup } from "./composite-catalog-group";

const clip = (hash: string, name: string, over: Partial<ClipDTO> = {}): ClipDTO => ({
  hash,
  name,
  kind: "commercial",
  era: 1990,
  audience: "general",
  category: "drinks",
  durationMs: 120_000,
  source: "folder",
  playCount: 0,
  playsCounted: true,
  aiTagged: false,
  tagged: true,
  needsDetails: false,
  suggestedEra: 0,
  ...over,
});

const reel = clip("reel-hash", "Saturday reel", { isComposite: true });

const renderGroup = (onManage = vi.fn()) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <CompositeCatalogGroup
        clip={reel}
        onManage={onManage}
        renderParent={(c) => <p>parent: {c.name}</p>}
        renderChild={(c) => <p key={c.hash}>child: {c.name}</p>}
      />
    </QueryClientProvider>,
  );

describe("CompositeCatalogGroup", () => {
  it("renders the clip as the parent and does not load segments until opened", () => {
    const requests: string[] = [];
    server.use(
      getListFillerMockHandler(({ request }) => {
        requests.push(request.url);
        return { clips: [], total: 0 };
      }),
    );
    renderGroup();

    expect(screen.getByText("parent: Saturday reel")).toBeVisible();
    expect(screen.getByText("2m source reel")).toBeVisible();
    expect(requests).toEqual([]);
  });

  it("loads the compilation's own segments when opened, and offers to manage them", async () => {
    const urls: URL[] = [];
    const onManage = vi.fn();
    server.use(
      getListFillerMockHandler(({ request }) => {
        urls.push(new URL(request.url));
        return { clips: [clip("seg-1", "Soda spot"), clip("seg-2", "Cereal spot")], total: 2 };
      }),
    );
    renderGroup(onManage);

    await userEvent.click(screen.getByRole("button", { name: "Show segments from Saturday reel" }));

    expect(await screen.findByText("child: Soda spot")).toBeVisible();
    expect(screen.getByText("child: Cereal spot")).toBeVisible();
    expect(urls[0]?.searchParams.get("parentHash")).toBe("reel-hash");
    await userEvent.click(await screen.findByRole("button", { name: "Manage 2 segments" }));
    expect(onManage).toHaveBeenCalledOnce();
  });

  it("says so when no segments remain filed", async () => {
    server.use(getListFillerMockHandler({ clips: [], total: 0 }));
    renderGroup();

    await userEvent.click(screen.getByRole("button", { name: "Show segments from Saturday reel" }));

    expect(await screen.findByText("No filed segments remain under this compilation.")).toBeVisible();
  });

  it("reports a segment list that could not be loaded", async () => {
    server.use(
      http.get("*/v1/filler", () =>
        HttpResponse.json({ title: "Unavailable", status: 503 }, { status: 503 }),
      ),
    );
    renderGroup();

    await userEvent.click(screen.getByRole("button", { name: "Show segments from Saturday reel" }));

    expect(await screen.findByText("Segments could not be loaded.")).toBeVisible();
  });
});
