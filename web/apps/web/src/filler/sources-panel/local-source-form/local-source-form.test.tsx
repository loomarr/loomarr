import { getAddFillerSourceMockHandler, getListFillerProvidersMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { fillerProviders } from "@/test/fixtures/filler";
import { server } from "@/test/msw/server";
import { LocalSourceForm } from "./local-source-form";

const renderForm = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <LocalSourceForm />
    </QueryClientProvider>,
  );

beforeEach(() => {
  server.use(getListFillerProvidersMockHandler({ providers: fillerProviders }));
});

// The kinds on offer, their names, the field's label and its example all come from the provider
// registry, so the form carries no per-kind copy of its own.
describe("LocalSourceForm", () => {
  it("starts on the registry's first typed-address provider and switches the field copy with the kind", async () => {
    renderForm();

    expect(await screen.findByRole("textbox", { name: "Folder path" })).toHaveAttribute(
      "placeholder",
      expect.stringContaining("a full path"),
    );

    await userEvent.click(screen.getByRole("combobox", { name: "Kind of source to add" }));
    await userEvent.click(await screen.findByRole("option", { name: "Media library" }));

    expect(screen.getByRole("textbox", { name: "Library name" })).toHaveAttribute(
      "placeholder",
      expect.stringContaining("Commercials"),
    );
  });

  it("offers a provider the page has never heard of, from its description alone", async () => {
    server.use(
      getListFillerProvidersMockHandler({
        providers: [
          ...fillerProviders,
          {
            id: "share",
            label: "Network share",
            addressHint: "//nas/ads",
            addressLabel: "Share path",
            fetched: false,
            listable: false,
            searchable: false,
            discoverable: false,
            openSearchable: false,
            addable: true,
          },
        ],
      }),
    );
    renderForm();

    await userEvent.click(await screen.findByRole("combobox", { name: "Kind of source to add" }));
    await userEvent.click(await screen.findByRole("option", { name: "Network share" }));
    expect(screen.getByRole("textbox", { name: "Share path" })).toHaveAttribute("placeholder", "//nas/ads");
  });

  it("offers nothing for a provider that is fetched or cannot be typed in", async () => {
    server.use(
      getListFillerProvidersMockHandler({
        providers: fillerProviders.map((provider) => ({ ...provider, addable: false })),
      }),
    );
    renderForm();
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByRole("combobox", { name: "Kind of source to add" })).not.toBeInTheDocument();
  });

  it("sends the trimmed address with the chosen kind and clears the field", async () => {
    const adds: unknown[] = [];
    server.use(
      getAddFillerSourceMockHandler(async ({ request }) => {
        adds.push(await request.json());
        return { id: "src-new", label: "New source", uri: "/mnt/extra-ads", enabled: true };
      }),
    );
    renderForm();
    const add = await screen.findByRole("button", { name: "+ Add source" });
    expect(add).toBeDisabled();

    await userEvent.type(screen.getByRole("textbox", { name: "Folder path" }), "  /mnt/extra-ads  ");
    await userEvent.click(add);

    await waitFor(() => expect(adds).toEqual([{ kind: "folder", uri: "/mnt/extra-ads" }]));
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Folder path" })).toHaveValue(""));
  });
});
