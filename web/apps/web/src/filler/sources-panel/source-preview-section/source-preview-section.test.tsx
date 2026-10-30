import { getResolveFillerSourceMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { fillerSource } from "@/test/fixtures/filler";
import { server } from "@/test/msw/server";
import { registeredSourceURL, SourcePreviewSection } from "./source-preview-section";

const renderSection = (source: ReturnType<typeof fillerSource>) =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <SourcePreviewSection source={source} />
    </QueryClientProvider>,
  );

describe("registeredSourceURL", () => {
  it("turns an archive identifier into its details page and leaves full URLs alone", () => {
    expect(registeredSourceURL(fillerSource({ kind: "archive", uri: "classic tv" }))).toBe(
      "https://archive.org/details/classic%20tv",
    );
    expect(registeredSourceURL(fillerSource({ kind: "archive", uri: "https://archive.org/details/x" }))).toBe(
      "https://archive.org/details/x",
    );
    expect(registeredSourceURL(fillerSource({ kind: "youtube", uri: "https://www.youtube.com/@ads" }))).toBe(
      "https://www.youtube.com/@ads",
    );
  });

  it("has no address for local sources or a blank target", () => {
    expect(registeredSourceURL(fillerSource({ kind: "folder", uri: "/data/ads" }))).toBeUndefined();
    expect(registeredSourceURL(fillerSource({ kind: "archive", uri: "  " }))).toBeUndefined();
  });
});

describe("SourcePreviewSection", () => {
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
    renderSection(fillerSource({ id: "archive:classic", kind: "archive", uri: "classic_tv" }));

    expect(screen.getByRole("heading", { name: "From this source" })).toBeVisible();
    expect(await screen.findByText("First station break")).toBeVisible();
    expect(resolutions).toEqual([{ kind: "archive", input: "classic_tv" }]);
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
        providerEnabled: false,
      }),
    );

    expect(await screen.findByText("Resume YouTube to refresh these examples.")).toBeVisible();
    await waitFor(() => expect(resolved).toBe(false));
  });

  it("renders nothing for a local source", () => {
    renderSection(fillerSource({ kind: "folder", uri: "/data/ads" }));
    expect(screen.queryByRole("heading", { name: "From this source" })).not.toBeInTheDocument();
  });
});
