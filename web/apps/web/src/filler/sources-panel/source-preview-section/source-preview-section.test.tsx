import { getListFillerProvidersMockHandler, getResolveFillerSourceMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { fillerProviders, fillerSource } from "@/test/fixtures/filler";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { SourcePreviewSection } from "./source-preview-section";

const renderSection = (source: ReturnType<typeof fillerSource>) =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <SourcePreviewSection source={source} />
    </QueryClientProvider>,
  );

describe("SourcePreviewSection", () => {
  beforeEach(() => {
    server.use(getListFillerProvidersMockHandler({ providers: fillerProviders }));
  });

  it("resolves the registered source once and shows its examples", async () => {
    const resolutions: { kind: string; input: string }[] = [];
    server.use(
      getResolveFillerSourceMockHandler(async ({ request, params }) => {
        const body = (await request.json()) as { input: string };
        resolutions.push({ kind: String(params.kind), input: body.input });
        return {
          provider: "archive",
          targetType: "collection",
          canonicalId: body.input,
          canonicalUrl: `https://archive.org/details/${body.input}`,
          title: "Classic TV",
          alreadyAdded: true,
          previewItems: [{ title: "First station break", url: "https://archive.org/details/break_one" }],
        };
      }),
    );
    renderSection(
      fillerSource({
        id: "archive:classic",
        kind: "archive",
        uri: "classic_tv",
        pageUrl: "https://archive.org/details/classic_tv",
      }),
    );

    expect(await screen.findByRole("heading", { name: "From this source" })).toBeVisible();
    expect(await screen.findByText("First station break")).toBeVisible();
    expect(screen.getByText("Popular videos in this collection")).toBeVisible();
    expect(resolutions).toEqual([{ kind: "archive", input: "classic_tv" }]);
  });

  it("links out to the page the server gave the source, whichever provider it is", async () => {
    server.use(
      getListFillerProvidersMockHandler({
        providers: [
          ...fillerProviders,
          {
            id: "thirdparty",
            label: "Third Party",
            fetched: true,
            listable: true,
            searchable: false,
            discoverable: true,
            openSearchable: false,
            addable: true,
            previewHeading: "Samples from Third Party",
          },
        ],
      }),
      getResolveFillerSourceMockHandler(() => {
        throw new Error("a resolve is not what this asserts");
      }),
    );
    renderSection(
      fillerSource({
        id: "thirdparty:one",
        kind: "thirdparty",
        uri: "one",
        pageUrl: "https://third.example/one",
        providerEnabled: false,
      }),
    );

    expect(await screen.findByRole("link", { name: /Open on Third Party/ })).toHaveAttribute(
      "href",
      "https://third.example/one",
    );
    expect(screen.getByText("Resume Third Party to refresh these examples.")).toBeVisible();
  });

  it("asks to resume a paused provider instead of resolving", async () => {
    let resolved = false;
    server.use(
      getResolveFillerSourceMockHandler(() => {
        resolved = true;
        throw new Error("a paused provider must not be resolved");
      }),
    );
    renderSection(
      fillerSource({
        id: "youtube:ads",
        kind: "youtube",
        uri: "https://www.youtube.com/@ads",
        pageUrl: "https://www.youtube.com/@ads",
        providerEnabled: false,
      }),
    );

    expect(await screen.findByText("Resume YouTube to refresh these examples.")).toBeVisible();
    await waitFor(() => expect(resolved).toBe(false));
  });

  it("renders nothing for a source with no page of its own", () => {
    renderSection(fillerSource({ kind: "folder", uri: "/data/ads" }));
    expect(screen.queryByRole("heading", { name: "From this source" })).not.toBeInTheDocument();
  });
});
