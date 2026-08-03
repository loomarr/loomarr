import type { FillerSourceDTO } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SourcesPanel } from "./sources-panel";

// SourcesPanel owns the Sources tab's own state and mutations — the switches, the add form, the
// per-source search. It was extracted out of `filler-page.tsx` when that file reached ~1100 lines
// and every keystroke in the source-search box re-rendered the whole clip grid.
//
// ⚠ `sources`/`total` arrive as PROPS rather than from a query here: the page header's status
// line reads the same list, and a second `useListFillerSources` inside this panel would
// double-fetch it. These tests pass them directly, which is also what makes them fast.

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// Records method + url + parsed body, so a test can prove WHAT was sent rather than only that
// something was.
// ⚠ `/v1/auth/me` must answer with an ADMIN — note the path, it is not `/v1/me`. The panel reads
// `useAuth()`, and the add form, the switches and the per-source search are all admin-only, so a
// stub that misses this route renders a read-only list and every admin assertion fails for the
// wrong reason.
const stubFetch = () => {
  const calls: { method: string; url: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      const u = String(url);
      calls.push({
        method,
        url: u,
        body: init?.body ? JSON.parse(init.body as string) : undefined,
      });
      if (u.includes("/v1/auth/me")) {
        return Promise.resolve(
          jsonResponse(200, {
            id: "u1",
            name: "Admin",
            role: "admin",
            autoApprove: true,
            disabled: false,
            quota: 0,
          }),
        );
      }
      return Promise.resolve(jsonResponse(200, { sources: [], total: 0, results: [] }));
    }),
  );
  return calls;
};

const source = (over: Partial<FillerSourceDTO> & Pick<FillerSourceDTO, "kind">): FillerSourceDTO => ({
  id: over.kind,
  target: "/data/filler",
  detail: "watched directly",
  count: 0,
  configured: true,
  fetchable: true,
  enabled: true,
  switchable: true,
  removable: false,
  searchable: false,
  ...over,
});

const renderPanel = (sources: FillerSourceDTO[] = [source({ kind: "folder" })]) =>
  render(<SourcesPanel sources={sources} />, { wrapper: makeWrapper() });

afterEach(() => vi.unstubAllGlobals());

describe("SourcesPanel", () => {
  it("lists the sources it is given", () => {
    stubFetch();
    renderPanel([
      source({ kind: "folder", id: "folder", target: "/data/filler" }),
      source({ kind: "archive", id: "archive:classic", target: "classic_tv_commercials" }),
    ]);

    expect(screen.getByText("/data/filler")).toBeInTheDocument();
    expect(screen.getByText("classic_tv_commercials")).toBeInTheDocument();
  });

  // ⚠ The kind is EXPLICIT on the wire, not sniffed from the URI. An archive identifier and a
  // YouTube playlist URL are validated by incompatible rules on the server — before the kind was
  // sent, the hardcoded archive validator rejected every playlist URL with a message about
  // archive.org collections.
  it("sends the chosen kind with the new source", async () => {
    const calls = stubFetch();
    renderPanel();

    // The add form is admin-only, so it appears only once `/v1/me` has resolved.
    const uri = await screen.findByRole("textbox");
    await userEvent.type(uri, "/mnt/extra-ads");
    await userEvent.click(screen.getByRole("button", { name: /add source/i }));

    await waitFor(() => {
      const post = calls.find((c) => c.method === "POST" && c.url.includes("/v1/filler/sources"));
      expect(post?.body).toEqual({ kind: "library", uri: "/mnt/extra-ads" });
    });
  });

  // ⚠ THE promise the switch has to keep. Switching a source off withdraws it from future
  // scanning; it does NOT remove clips already in the catalog. An operator who reads "off" as
  // "my clips are gone" will switch it back on and re-download everything they already have.
  it("switching a source off sends only the enabled flag", async () => {
    const calls = stubFetch();
    renderPanel([source({ kind: "folder", id: "folder", target: "/data/filler" })]);

    await userEvent.click(screen.getByRole("switch", { name: "Use /data/filler" }));

    await waitFor(() => {
      const patch = calls.find((c) => c.method === "PATCH");
      expect(patch?.url).toContain("/v1/filler/sources/folder");
      expect(patch?.body).toMatchObject({ enabled: false });
    });
  });

  // ⚠ Only `archive` can be searched in place, and the panel reads the server's `searchable`
  // flag rather than testing the kind itself. A YouTube playlist can only be ENUMERATED by
  // yt-dlp, so a search box there would return nothing forever.
  it("offers search only on a searchable source", () => {
    stubFetch();
    const { unmount } = renderPanel([source({ kind: "archive", id: "a", searchable: true })]);
    expect(screen.getByRole("button", { name: /search it/i })).toBeInTheDocument();
    unmount();

    renderPanel([source({ kind: "youtube", id: "y", searchable: false })]);
    expect(screen.queryByRole("button", { name: /search it/i })).not.toBeInTheDocument();
  });

  // Registering records that a source EXISTS and is allowed. Downloading is the pull's approval
  // gate or a deliberate per-result queue — an operator who expects "add" to start fetching would
  // otherwise read the silence as a failure.
  it("says that adding a source downloads nothing", async () => {
    stubFetch();
    renderPanel();
    expect(await screen.findByText(/downloads nothing/i)).toBeInTheDocument();
  });
});
