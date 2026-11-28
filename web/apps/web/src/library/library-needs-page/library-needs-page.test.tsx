import { getIgnoreLibraryItemMockHandler, getLibraryStatusMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LoomarrEventsProvider } from "@/events/events-provider";
import { libraryStatus } from "@/test/fixtures/library";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { LibraryNeedsPage } from "./library-needs-page";

const mocks = vi.hoisted(() => ({ navigate: vi.fn() }));
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => mocks.navigate }));
vi.mock("sonner", () => ({ toast: Object.assign(vi.fn(), { error: vi.fn() }) }));

const need = (id: string) => ({
  item: { id, kind: "movie" as const, name: id, ready: true },
  searched: true,
  candidates: [],
  files: [{ path: `/media/Tapes/${id}.mkv` }],
  folder: "Tapes",
});
const ignored = (id: string) => ({
  id,
  kind: "movie" as const,
  name: `${id}.mkv`,
  folder: "Tapes",
  ignoredAt: "2026-01-02T03:04:05Z",
});

// A library of two waiting tapes: ignoring takes them off the list, restoring puts them back.
const sent = { batch: [] as unknown[], restore: [] as unknown[] };
let gone: string[] = [];
const serve = () => {
  server.use(
    getLibraryStatusMockHandler(libraryStatus()),
    http.get("*/v1/library/needs", () =>
      HttpResponse.json({
        needs: ["one", "two"].filter((id) => !gone.includes(id)).map(need),
        orders: [],
        problems: [],
      }),
    ),
    getIgnoreLibraryItemMockHandler(),
    http.post("*/v1/library/needs/ignore", async ({ request }) => {
      const body = (await request.json()) as { ids: string[] };
      sent.batch.push(body);
      gone = body.ids;
      return HttpResponse.json({ ids: body.ids });
    }),
    http.post("*/v1/library/ignored/restore", async ({ request }) => {
      sent.restore.push(await request.json());
      gone = [];
      return new HttpResponse(null, { status: 204 });
    }),
    http.get("*/v1/library/ignored", ({ request }) => {
      const offset = Number(new URL(request.url).searchParams.get("offset") ?? 0);
      return HttpResponse.json({ items: [ignored(offset === 0 ? "newest" : "older")], total: 2 });
    }),
  );
};

const renderPage = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <LoomarrEventsProvider>
        <LibraryNeedsPage />
      </LoomarrEventsProvider>
    </QueryClientProvider>,
  );
};

const pressUndo = async () => {
  await waitFor(() => expect(toast).toHaveBeenCalled());
  const options = vi.mocked(toast).mock.calls[0]?.[1] as unknown as { action: { onClick: () => void } };
  await act(async () => options.action.onClick());
};

describe("LibraryNeedsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    gone = [];
    sent.batch = [];
    sent.restore = [];
    serve();
  });

  it("has its own header, a way back and the two tabs with counts", async () => {
    renderPage();
    expect(await screen.findByRole("heading", { name: "Needs you" })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("tab", { name: /Waiting\s*2/ })).toBeInTheDocument());
    expect(screen.getByRole("tab", { name: /Ignored\s*2/ })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Library" }));
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library" });
  });

  it("ignores a folder's shown titles in one go and undoes it", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Ignore these 2" }));
    await pressUndo();
    expect(sent.batch).toEqual([{ ids: ["one", "two"] }]);
    expect(vi.mocked(toast).mock.calls[0]?.[0]).toBe("Ignored 2 in Tapes");
    await waitFor(() => expect(sent.restore).toEqual([{ ids: ["one", "two"] }]));
    expect(await screen.findByRole("button", { name: "Ignore these 2" })).toBeInTheDocument();
  });

  it("undoes a single ignore and shows the card again", async () => {
    renderPage();
    const [first] = await screen.findAllByRole("button", { name: "Ignore this" });
    if (!first) throw new Error("no card");
    await userEvent.click(first);
    await pressUndo();
    expect(vi.mocked(toast).mock.calls[0]?.[0]).toBe("Ignored one.mkv");
    await waitFor(() => expect(sent.restore).toEqual([{ ids: ["one"] }]));
    await waitFor(() => expect(screen.getAllByRole("button", { name: "Ignore this" })).toHaveLength(2));
  });

  it("lists the ignored titles page by page and brings one back", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("tab", { name: /Ignored/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Show more" }));
    expect(await screen.findByRole("button", { name: "Bring back older.mkv" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Bring back newest.mkv" }));
    await waitFor(() => expect(sent.restore).toEqual([{ ids: ["newest"] }]));
  });
});
