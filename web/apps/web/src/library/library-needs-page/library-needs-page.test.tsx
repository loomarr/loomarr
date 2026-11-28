import { getIgnoreLibraryItemMockHandler, getLibraryStatusMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
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
    expect(vi.mocked(toast).mock.calls[0]?.[0]).toBe("Ignored 2 files in Tapes");
    await waitFor(() => expect(sent.restore).toEqual([{ ids: ["one", "two"] }]));
    expect(await screen.findByRole("button", { name: "Ignore these 2" })).toBeInTheDocument();
  });

  it("undoes a single ignore and shows the card again", async () => {
    renderPage();
    const [first] = await screen.findAllByRole("button", { name: /^Ignore this: / });
    if (!first) throw new Error("no card");
    await userEvent.click(first);
    // Focus moves to the next card's first action rather than dropping to the page.
    expect(document.activeElement).toHaveAccessibleName(/: (?!one\.mkv).+/);
    expect(document.activeElement).toHaveAttribute("role", "button");
    await pressUndo();
    expect(vi.mocked(toast).mock.calls[0]?.[0]).toBe("Ignored one.mkv");
    await waitFor(() => expect(sent.restore).toEqual([{ ids: ["one"] }]));
    await waitFor(() => expect(screen.getAllByRole("button", { name: /^Ignore this: / })).toHaveLength(2));
  });

  it("keeps a title with no guess and undoes it once, however often Undo is pressed", async () => {
    const kept: string[] = [];
    const undone: string[] = [];
    server.use(
      http.post("*/v1/library/items/:id/keep", ({ params }) => {
        kept.push(String(params.id));
        gone = [String(params.id)];
        return new HttpResponse(null, { status: 204 });
      }),
      http.delete("*/v1/library/items/:id/answer", ({ params }) => {
        undone.push(String(params.id));
        gone = [];
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPage();
    const [first] = await screen.findAllByRole("button", { name: /^It's ours, keep it as is: / });
    if (!first) throw new Error("no card");
    await userEvent.click(first);
    await pressUndo();
    await pressUndo();
    expect(kept).toEqual(["one"]);
    expect(vi.mocked(toast).mock.calls[0]?.[0]).toBe("Kept one.mkv");
    expect(vi.mocked(toast).mock.calls[0]?.[1]).toMatchObject({
      duration: 30_000,
      actionButtonStyle: { height: 44, minWidth: 44 },
    });
    await waitFor(() => expect(undone).toEqual(["one"]));
    await waitFor(() =>
      expect(screen.getAllByRole("button", { name: /^It's ours, keep it as is: / })).toHaveLength(2),
    );
  });

  it("says why an Undo of a keep was refused, in the server's words", async () => {
    server.use(
      http.post("*/v1/library/items/:id/keep", () => new HttpResponse(null, { status: 204 })),
      http.delete("*/v1/library/items/:id/answer", () =>
        HttpResponse.json({ title: "Nothing to undo", detail: "It was changed since." }, { status: 409 }),
      ),
    );
    renderPage();
    const [first] = await screen.findAllByRole("button", { name: /^It's ours, keep it as is: / });
    if (!first) throw new Error("no card");
    await userEvent.click(first);
    await pressUndo();
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Nothing to undo", {
        description: "It was changed since.",
        duration: 30_000,
      }),
    );
  });

  it("offers no keep on a card with a guess, even one whose name doesn't agree", async () => {
    const guess = {
      externalId: { provider: "tmdb", id: "1" },
      kind: "movie",
      name: "Made Up Title",
      nameMatches: false,
      yearMatches: false,
      lengthMatches: false,
      episodesMatches: false,
    };
    server.use(
      http.get("*/v1/library/needs", () =>
        HttpResponse.json({
          needs: [
            { ...need("one"), candidates: [guess] },
            { ...need("two"), candidates: [{ ...guess, nameMatches: true }] },
          ],
          orders: [],
          problems: [],
        }),
      ),
    );
    renderPage();
    await screen.findByRole("button", { name: /^Find it: / });
    expect(screen.queryByRole("button", { name: /^It's ours, keep it as is: / })).not.toBeInTheDocument();
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

  const openIgnored = async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("tab", { name: /Ignored/ }));
    return screen.findByRole("button", { name: "Bring back newest.mkv" });
  };

  it("sends one restore however often Bring back is pressed", async () => {
    const button = await openIgnored();
    act(() => {
      fireEvent.click(button);
      fireEvent.click(button);
    });
    await waitFor(() => expect(sent.restore).toHaveLength(1));
    await new Promise((done) => setTimeout(done, 50));
    expect(sent.restore).toHaveLength(1);
  });

  it("says why Bring back was refused and drops the stale row", async () => {
    const button = await openIgnored();
    server.use(
      http.post("*/v1/library/ignored/restore", () =>
        HttpResponse.json({ title: "Already back", detail: "It is waiting again." }, { status: 409 }),
      ),
      http.get("*/v1/library/ignored", () => HttpResponse.json({ items: [], total: 0 })),
    );
    await userEvent.click(button);
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Already back", {
        description: "It is waiting again.",
        duration: 30_000,
      }),
    );
    await waitFor(() => expect(button).not.toBeInTheDocument());
  });

  it("words a failed Bring back as that, not as an undo", async () => {
    const button = await openIgnored();
    server.use(http.post("*/v1/library/ignored/restore", () => HttpResponse.error()));
    await userEvent.click(button);
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    const [title, options] = vi.mocked(toast.error).mock.calls[0] ?? [];
    expect(title).toBe("Couldn't bring it back");
    expect((options as { action: { label: string } }).action.label).toBe("Try again");
  });

  it("counts a title in two lists once, as the one card it draws", async () => {
    const item = { id: "one", kind: "movie", name: "Low Tide", ready: true, unplayable: 1 };
    server.use(
      http.get("*/v1/library/needs", () =>
        HttpResponse.json({ needs: [need("one")], orders: [], problems: [{ item, extraBytes: 10 }] }),
      ),
    );
    renderPage();
    expect(await screen.findByRole("tab", { name: /Waiting\s*1/ })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /^See why/ })).toHaveLength(1);
    expect(screen.queryByText("Needs a look")).not.toBeInTheDocument();
  });

  it("counts waiting titles, not the reasons a title needs a look", async () => {
    const item = { id: "bad", kind: "movie", name: "Low Tide", ready: true, unplayable: 1 };
    server.use(
      http.get("*/v1/library/needs", () =>
        HttpResponse.json({ needs: [], orders: [], problems: [{ item, extraBytes: 10 }] }),
      ),
    );
    renderPage();
    expect(await screen.findByRole("tab", { name: /Waiting\s*1/ })).toBeInTheDocument();
  });

  it("leaves other cards as they were when one is restored", async () => {
    renderPage();
    const [first] = await screen.findAllByRole("button", { name: /^Ignore this: / });
    const [, second] = screen.getAllByRole("button", { name: /^Find it: / });
    if (!first || !second) throw new Error("no cards");
    await userEvent.click(second);
    await userEvent.click(first);
    await pressUndo();
    await waitFor(() => expect(screen.getAllByRole("button", { name: /^Ignore this: / })).toHaveLength(2));
    expect(screen.getByLabelText("Search for the title")).toBeInTheDocument();
  });

  it("says All caught up when nothing waits, with the way back and to the ignored", async () => {
    gone = ["one", "two"];
    renderPage();
    expect(await screen.findByRole("heading", { name: "All caught up" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Back to the Library" }));
    expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library" });
    await userEvent.click(await screen.findByRole("button", { name: "2 ignored files" }));
    expect(await screen.findByRole("button", { name: "Bring back newest.mkv" })).toBeInTheDocument();
  });
});
