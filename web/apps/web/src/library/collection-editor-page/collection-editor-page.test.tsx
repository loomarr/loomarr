import { getGetCollectionMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { CollectionEditorPage } from "./collection-editor-page";

const mocks = vi.hoisted(() => ({ navigate: vi.fn() }));
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => mocks.navigate }));

const title = (id: string, name: string, year?: number) => ({
  id,
  kind: "movie" as const,
  name,
  year,
  files: 1,
  ready: true,
  unplayable: 0,
  extraCopies: 0,
});

const collection = (usedBy: { id: string; name: string; number: number }[] = []) => ({
  id: "c1",
  name: "Rainy Sundays",
  description: "Slow ones",
  titles: [title("90000801", "Harbour Lights", 1999), title("90000802", "Paper Kites")],
  usedBy,
});

const renderPage = (id?: string) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <CollectionEditorPage id={id} />
    </QueryClientProvider>,
  );
};

// The wiring: the editor opens on the saved collection, saves the name, description and titles in
// order together, adds a title the library search finds, and deletes only when no channel uses it.
describe("CollectionEditorPage", () => {
  beforeEach(() => mocks.navigate.mockReset());

  it("saves the edited collection with its titles in order, then goes back", async () => {
    let sent: unknown;
    server.use(
      getGetCollectionMockHandler(collection()),
      http.put("*/v1/collections/c1", async ({ request }) => {
        sent = await request.json();
        return HttpResponse.json(collection());
      }),
    );
    renderPage("c1");

    await userEvent.click(await screen.findByRole("button", { name: "Remove Harbour Lights" }));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(sent).toEqual({ name: "Rainy Sundays", description: "Slow ones", titles: ["90000802"] }),
    );
    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library/collections" }));
  });

  it("makes a new collection from a name and a title the search finds", async () => {
    let sent: unknown;
    server.use(
      http.get("*/v1/library/items", () =>
        HttpResponse.json({ items: [title("90000803", "Night Ferry", 2004)], total: 1 }),
      ),
      http.post("*/v1/collections", async ({ request }) => {
        sent = await request.json();
        return HttpResponse.json(collection(), { status: 201 });
      }),
    );
    renderPage();

    await userEvent.type(screen.getByLabelText("Name"), "Ferries");
    await userEvent.type(screen.getByLabelText("Add a title"), "night");
    await userEvent.click(await screen.findByRole("button", { name: "Add Night Ferry" }));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(sent).toEqual({ name: "Ferries", description: "", titles: ["90000803"] }));
  });

  it("says when a save didn't work, and stays", async () => {
    server.use(
      getGetCollectionMockHandler(collection()),
      http.put("*/v1/collections/c1", () =>
        HttpResponse.json(
          { title: "Boom", status: 500 },
          { status: 500, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    renderPage("c1");

    await userEvent.click(await screen.findByRole("button", { name: "Save" }));
    expect(await screen.findByText("Couldn't save just now. Try again in a moment.")).toBeInTheDocument();
    expect(mocks.navigate).not.toHaveBeenCalled();
  });

  it("won't delete while a channel uses it", async () => {
    server.use(
      getGetCollectionMockHandler(collection([{ id: "90000901", name: "Saturday Morning", number: 4 }])),
    );
    renderPage("c1");

    expect(
      await screen.findByText("On Saturday Morning (channel 4) uses this. Take it off the channel first."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete collection" })).toBeDisabled();
  });

  it("deletes one no channel uses", async () => {
    let deleted = false;
    server.use(
      getGetCollectionMockHandler(collection()),
      http.delete("*/v1/collections/c1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPage("c1");

    await userEvent.click(await screen.findByRole("button", { name: "Delete collection" }));
    expect(deleted).toBe(false);
    await userEvent.click(await screen.findByRole("button", { name: "Delete" }));
    await waitFor(() => expect(deleted).toBe(true));
    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledWith({ to: "/library/collections" }));
  });
});
