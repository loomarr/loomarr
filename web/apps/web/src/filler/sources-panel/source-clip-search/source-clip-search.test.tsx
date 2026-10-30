import { getDiscoverFillerMockHandler, getMeMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { fillerSource } from "@/test/fixtures/filler";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { SourceClipSearch } from "./source-clip-search";

const archive = fillerSource({
  id: "archive:classic",
  kind: "archive",
  uri: "classic_tv",
  target: "Classic TV Commercials",
  searchable: true,
  actions: ["fetch", "search"],
});

const renderSearch = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <SourceClipSearch source={archive} />
    </QueryClientProvider>,
  );

describe("SourceClipSearch", () => {
  it("searches the source's own collection only once a query is submitted", async () => {
    const discoveries: URL[] = [];
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getDiscoverFillerMockHandler(({ request }) => {
        discoveries.push(new URL(request.url));
        return { items: [], total: 0, licenceNote: "Check licences." };
      }),
    );
    renderSearch();

    await userEvent.click(screen.getByRole("button", { name: /find a specific clip/i }));
    await userEvent.type(screen.getByRole("textbox", { name: /search this source/i }), "cereal");
    expect(discoveries).toHaveLength(0);
    await userEvent.click(screen.getByRole("button", { name: /^search$/i }));

    await waitFor(() => expect(discoveries).toHaveLength(1));
    expect(discoveries[0]?.searchParams.get("q")).toBe("cereal");
    expect(discoveries[0]?.searchParams.get("collection")).toBe("classic_tv");
  });

  it("lists the hits it finds", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getDiscoverFillerMockHandler({
        items: [
          {
            id: "cereal_ad",
            title: "Cereal commercial, 1984",
            url: "https://archive.org/details/cereal_ad",
          },
        ],
        total: 1,
        licenceNote: "Check licences.",
      }),
    );
    renderSearch();

    await userEvent.click(screen.getByRole("button", { name: /find a specific clip/i }));
    await userEvent.type(screen.getByRole("textbox", { name: /search this source/i }), "cereal");
    await userEvent.click(screen.getByRole("button", { name: /^search$/i }));

    expect(await screen.findByText("Cereal commercial, 1984")).toBeVisible();
  });
});
