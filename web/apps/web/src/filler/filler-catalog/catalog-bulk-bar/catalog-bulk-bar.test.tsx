import { getBulkRemoveFillerMockHandler, getBulkTagFillerMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { BULK_TAG_FIELDS, CatalogBulkBar } from "./catalog-bulk-bar";

const renderBar = (onClear = vi.fn(), selected = new Set(["hash-a", "hash-b"])) =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <CatalogBulkBar selected={selected} onClear={onClear} />
    </QueryClientProvider>,
  );

describe("CatalogBulkBar", () => {
  it("counts the selection", () => {
    renderBar();
    expect(screen.getByText("2 clips selected")).toBeVisible();
  });

  it("offers only the closed-enum fields a menu can express", () => {
    expect(BULK_TAG_FIELDS.map((field) => field.key)).toEqual(["era", "audience"]);
  });

  it("retags with only the field picked, keyed by hash, then clears the selection", async () => {
    const bodies: unknown[] = [];
    server.use(
      getBulkTagFillerMockHandler(async ({ request }) => {
        bodies.push(await request.json());
        return { missing: 0, updated: 2 };
      }),
    );
    const onClear = vi.fn();
    renderBar(onClear);

    await userEvent.click(screen.getByRole("combobox", { name: "Set era" }));
    await userEvent.click(await screen.findByRole("option", { name: "1980" }));

    await waitFor(() => expect(bodies).toEqual([{ hashes: ["hash-a", "hash-b"], era: 1980 }]));
    await waitFor(() => expect(onClear).toHaveBeenCalledOnce());
  });

  it("sends an audience as the enum value, not a number", async () => {
    const bodies: unknown[] = [];
    server.use(
      getBulkTagFillerMockHandler(async ({ request }) => {
        bodies.push(await request.json());
        return { missing: 0, updated: 2 };
      }),
    );
    renderBar();

    await userEvent.click(screen.getByRole("combobox", { name: "Set audience" }));
    await userEvent.click(await screen.findByRole("option", { name: "kids" }));

    await waitFor(() => expect(bodies).toEqual([{ hashes: ["hash-a", "hash-b"], audience: "kids" }]));
  });

  it("removes from the catalog by hash and keeps the selection", async () => {
    const bodies: unknown[] = [];
    server.use(
      getBulkRemoveFillerMockHandler(async ({ request }) => {
        bodies.push(await request.json());
        return { missing: 0, updated: 2 };
      }),
    );
    const onClear = vi.fn();
    renderBar(onClear);

    await userEvent.click(screen.getByRole("button", { name: "Remove from catalog" }));

    await waitFor(() => expect(bodies).toEqual([{ hashes: ["hash-a", "hash-b"] }]));
    expect(onClear).not.toHaveBeenCalled();
  });

  it("clears on request", async () => {
    const onClear = vi.fn();
    renderBar(onClear);

    await userEvent.click(screen.getByRole("button", { name: "Clear" }));

    expect(onClear).toHaveBeenCalledOnce();
  });
});
