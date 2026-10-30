import type { ClipDTO } from "@loomarr/api";
import { getListFillerMockHandler, getListTaxonomyMockHandler } from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, type ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { RouterHarness } from "@/test/story-utils";
import { ClipInspectorSheet } from "./clip-inspector-sheet";

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
  needsDetails: false,
  suggestedEra: 0,
};

const Wrapper = ({ children }: { children: ReactNode }) => (
  <LoomarrProvider theme="dark">
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      {children}
    </QueryClientProvider>
  </LoomarrProvider>
);

const renderSheet = (props: { hash?: string; isAdmin?: boolean; onClose?: () => void }) =>
  render(
    <RouterHarness
      initialPath="/filler/library"
      content={
        <ClipInspectorSheet
          hash={props.hash}
          isAdmin={props.isAdmin ?? false}
          finalFocus={createRef()}
          onClose={props.onClose ?? vi.fn()}
        />
      }
    />,
    { wrapper: Wrapper },
  );

describe("ClipInspectorSheet", () => {
  it("stays closed without a hash", () => {
    renderSheet({});
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("resolves the clip by its exact hash, including held and composite clips", async () => {
    const urls: URL[] = [];
    server.use(
      getListFillerMockHandler(({ request }) => {
        urls.push(new URL(request.url));
        return { clips: [clip], total: 1 };
      }),
    );
    renderSheet({ hash: "catalog-hash" });

    const panel = await screen.findByRole("dialog", { name: "Local soda commercial" });
    expect(within(panel).getByText("Ready")).toBeVisible();
    expect(urls[0]?.searchParams.getAll("hashes")).toEqual(["catalog-hash"]);
    expect(urls[0]?.searchParams.get("includeHeld")).toBe("true");
    expect(urls[0]?.searchParams.get("includeComposites")).toBe("true");
  });

  it("labels a held clip as preparing and a recording as a recording", async () => {
    server.use(getListFillerMockHandler({ clips: [{ ...clip, held: true }], total: 1 }));
    const { unmount } = renderSheet({ hash: "catalog-hash" });
    expect(await screen.findByText("Preparing")).toBeVisible();
    unmount();

    server.use(getListFillerMockHandler({ clips: [{ ...clip, isComposite: true }], total: 1 }));
    renderSheet({ hash: "catalog-hash" });
    expect(await screen.findByText("Recording")).toBeVisible();
  });

  it("offers editing only to an admin, and returns to the details when it is cancelled", async () => {
    server.use(
      getListFillerMockHandler({ clips: [clip], total: 1 }),
      getListTaxonomyMockHandler({
        taxa: [],
        totalClips: 1,
        taggedClips: 1,
        unclassifiedClips: 0,
        axisCoverage: [],
      }),
    );
    renderSheet({ hash: "catalog-hash", isAdmin: true });

    await userEvent.click(await screen.findByRole("button", { name: "Edit details" }));
    expect(await screen.findByRole("region", { name: "Edit details: Local soda commercial" })).toBeVisible();
    expect(screen.getByText("Edit details")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("button", { name: "Edit details" })).toBeVisible();
  });

  it("gives a member the read-only details", async () => {
    server.use(getListFillerMockHandler({ clips: [clip], total: 1 }));
    renderSheet({ hash: "catalog-hash", isAdmin: false });

    expect(await screen.findByText("More about this clip")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Edit details" })).not.toBeInTheDocument();
  });

  it("says when the clip has gone", async () => {
    server.use(getListFillerMockHandler({ clips: [], total: 0 }));
    renderSheet({ hash: "gone-hash" });

    expect(await screen.findByText("This clip is no longer available.")).toBeVisible();
  });

  it("asks to close when dismissed", async () => {
    server.use(getListFillerMockHandler({ clips: [clip], total: 1 }));
    const onClose = vi.fn();
    renderSheet({ hash: "catalog-hash", onClose });
    await screen.findByRole("dialog");

    await userEvent.click(screen.getByRole("button", { name: "Close" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });
});
