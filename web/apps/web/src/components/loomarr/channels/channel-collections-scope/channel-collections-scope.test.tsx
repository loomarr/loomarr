import type { ChannelPolicy } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { ChannelCollectionsScope } from "./channel-collections-scope";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <TooltipProvider>{children}</TooltipProvider>
    </QueryClientProvider>
  );
};

const render = (ui: ReactElement) => rtlRender(ui, { wrapper: makeWrapper() });

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubCollections = (status: number, body: unknown) => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (typeof url === "string" && url.includes("/v1/library/collections")) {
        return Promise.resolve(jsonResponse(status, body));
      }
      return Promise.resolve(jsonResponse(200, {}));
    }),
  );
};

afterEach(() => vi.unstubAllGlobals());

const TWO = {
  collections: [
    { id: "bs-1", name: "Halloween", childCount: 12 },
    { id: "bs-2", name: "Criterion" },
  ],
};

const EMPTY_SCOPE: ChannelPolicy = { ordering: "shuffle", scope: {} };

describe("ChannelCollectionsScope", () => {
  it("lists the operator's collections as checkboxes", async () => {
    stubCollections(200, TWO);
    render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    expect(await screen.findByRole("checkbox", { name: /Halloween/ })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: /Criterion/ })).toBeInTheDocument();
  });

  it("reflects the current scope, and ticking one commits the id", async () => {
    stubCollections(200, TWO);
    const onChange = vi.fn();
    render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={onChange} />);

    const halloween = await screen.findByRole("checkbox", { name: /Halloween/ });
    expect(halloween).not.toBeChecked();
    await userEvent.click(halloween);

    // ⚠ The stored value is the opaque BoxSet ID, never the display name: the engine filters on
    // ids, so committing "Halloween" would match nothing and the channel would silently empty.
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ scope: expect.objectContaining({ collections: ["bs-1"] }) }),
    );
  });

  it("shows an already-scoped collection as checked", async () => {
    stubCollections(200, TWO);
    render(
      <ChannelCollectionsScope
        policy={{ ...EMPTY_SCOPE, scope: { collections: ["bs-2"] } }}
        onChange={vi.fn()}
      />,
    );

    expect(await screen.findByRole("checkbox", { name: /Criterion/ })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: /Halloween/ })).not.toBeChecked();
  });

  // ⚠ Unticking the last one must send [], not undefined. `collections` is omitempty, so
  // dropping the key leaves the previous restriction in place — the box would appear to clear
  // while the channel stayed filtered. Same trap as scope.series and runtimeMax.
  it("sends an empty array when the last collection is unticked", async () => {
    stubCollections(200, TWO);
    const onChange = vi.fn();
    render(
      <ChannelCollectionsScope
        policy={{ ...EMPTY_SCOPE, scope: { collections: ["bs-1"] } }}
        onChange={onChange}
      />,
    );

    await userEvent.click(await screen.findByRole("checkbox", { name: /Halloween/ }));
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ scope: expect.objectContaining({ collections: [] }) }),
    );
  });

  // "You have none" and "the feature is unavailable" are different answers, and an empty box
  // reads as a broken control rather than an explanation.
  it("explains an empty collection list rather than showing nothing", async () => {
    stubCollections(200, { collections: [] });
    render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    expect(await screen.findByText(/no collections yet/i)).toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });

  // ⚠ A failing media server must not read as "you have none" — that sends the operator off to
  // create a collection they may already have. Distinct from both the empty and the 501 states.
  it("distinguishes a load failure from having no collections", async () => {
    stubCollections(502, { title: "Couldn't load collections" });
    render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    expect(await screen.findByText(/Check the media server connection/i)).toBeInTheDocument();
    expect(screen.queryByText(/no collections yet/i)).not.toBeInTheDocument();
  });

  // 501 = no library configured. The control is not "empty", it is inapplicable — offering it
  // would advertise a narrowing that cannot be satisfied, and Connections is where that is fixed.
  it("renders nothing at all when no media library is configured", async () => {
    stubCollections(501, { title: "Collections aren't available" });
    const { container } = render(<ChannelCollectionsScope policy={EMPTY_SCOPE} onChange={vi.fn()} />);

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});
