import { getGetLibraryItemMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { LibraryTitlePage } from "./library-title-page";

const mocks = vi.hoisted(() => ({ navigate: vi.fn(), back: vi.fn(), canGoBack: vi.fn(() => false) }));
vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mocks.navigate,
  useRouter: () => ({ history: { canGoBack: mocks.canGoBack, back: mocks.back } }),
}));

const echo = {
  item: { id: "90000003", kind: "movie" as const, name: "Echo", year: 1999, files: 1, ready: true },
  overview: "A story.",
  genres: ["Drama"],
  files: [{ path: "/media/movies/Echo/Echo.mkv", size: 1_000_000, ready: true }],
  seasons: [],
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
  beforeEach(() => {
    mocks.navigate.mockReset();
    mocks.back.mockReset();
    mocks.canGoBack.mockReset().mockReturnValue(false);
  });

  it("shows the title the server describes", async () => {
    server.use(getGetLibraryItemMockHandler(echo));
    renderPage();

    expect(await screen.findByRole("heading", { name: "Echo" })).toBeInTheDocument();
    expect(screen.getByText("A story.")).toBeInTheDocument();
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

  it("shows why the title couldn't be read", async () => {
    server.use(
      http.get("*/v1/library/items/:id", () =>
        HttpResponse.json(
          { title: "Not found", status: 404 },
          { status: 404, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    renderPage();

    expect(await screen.findByRole("button", { name: /try again/i })).toBeInTheDocument();
  });
});
