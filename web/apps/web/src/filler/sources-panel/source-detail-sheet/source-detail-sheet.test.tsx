import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { fillerSource } from "@/test/fixtures/filler";
import type { SourceActions } from "../use-source-actions";
import { SourceDetailSheet, sourceKindLabel } from "./source-detail-sheet";

const actions = {
  error: undefined,
  fetchSource: { pending: false, error: null, run: vi.fn() },
  updating: false,
  removing: false,
  updateSource: vi.fn(),
  removeSource: vi.fn(),
} as unknown as SourceActions;

const renderSheet = (source?: ReturnType<typeof fillerSource>, onClose = vi.fn()) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <SourceDetailSheet source={source} actions={actions} finalFocus={createRef()} onClose={onClose} />
    </QueryClientProvider>,
  );

describe("sourceKindLabel", () => {
  it("describes each kind in an operator's words", () => {
    expect(sourceKindLabel(fillerSource({ kind: "archive" }))).toBe("Archive.org collection");
    expect(sourceKindLabel(fillerSource({ kind: "youtube" }))).toBe("YouTube channel or playlist");
    expect(sourceKindLabel(fillerSource({ kind: "library" }))).toBe("Media server library");
    expect(sourceKindLabel(fillerSource({ kind: "folder" }))).toBe("Folder on this server");
  });
});

describe("SourceDetailSheet", () => {
  it("stays closed without a source", () => {
    renderSheet();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("opens on a source with its title, kind and status", async () => {
    renderSheet(fillerSource({ id: "library:ads", kind: "library", target: "Ads" }));

    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("Ads");
    expect(dialog).toHaveTextContent("Media server library");
    expect(screen.getByRole("heading", { name: "Ready" })).toBeVisible();
  });

  it("calls the drop-in folder by its own name", async () => {
    renderSheet(fillerSource({ id: "folder", kind: "folder", target: "/data/filler" }));
    expect(await screen.findByRole("heading", { name: "Drop folder" })).toBeVisible();
  });

  it("asks to close when the drawer is dismissed", async () => {
    const onClose = vi.fn();
    renderSheet(fillerSource(), onClose);
    await screen.findByRole("dialog");

    await userEvent.keyboard("{Escape}");

    expect(onClose).toHaveBeenCalled();
  });
});
