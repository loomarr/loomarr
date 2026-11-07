import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { fillerSource } from "@/test/fixtures/filler";
import type { SourceActions } from "../use-source-actions";
import { SourceSettings } from "./source-settings";

const actionsWith = (over: Partial<SourceActions> = {}) =>
  ({
    updating: false,
    updatingSourceID: undefined,
    removing: false,
    removingSourceID: undefined,
    updateSource: vi.fn(),
    removeSource: vi.fn(),
    ...over,
  }) as unknown as SourceActions;

const renderSettings = (source: ReturnType<typeof fillerSource>, actions = actionsWith()) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <SourceSettings source={source} actions={actions} />
    </QueryClientProvider>,
  );

describe("SourceSettings", () => {
  it("renders nothing when the source offers no settings", () => {
    const { container } = renderSettings(fillerSource({ actions: ["fetch", "disable"] }));
    expect(container).toBeEmptyDOMElement();
  });

  it("removes the source it is open on, naming it for assistive tech", async () => {
    const actions = actionsWith();
    renderSettings(
      fillerSource({
        id: "library:ads",
        kind: "library",
        target: "Ads",
        removable: true,
        actions: ["remove"],
      }),
      actions,
    );

    await userEvent.click(screen.getByRole("button", { name: /source settings/i }));
    await userEvent.click(screen.getByRole("button", { name: "Remove Ads" }));

    expect(actions.removeSource).toHaveBeenCalledWith("library:ads");
    expect(screen.getByText("Clips already downloaded stay in your library.")).toBeVisible();
  });

  it("shows the removal as in progress only for the source being removed", async () => {
    renderSettings(
      fillerSource({ id: "library:ads", kind: "library", target: "Ads", actions: ["remove"] }),
      actionsWith({ removing: true, removingSourceID: "library:ads" }),
    );
    await userEvent.click(screen.getByRole("button", { name: /source settings/i }));

    expect(screen.getByRole("button", { name: "Remove Ads" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Remove Ads" })).toHaveTextContent("Removing…");
  });

  it("offers only the sections the source supports", async () => {
    renderSettings(
      fillerSource({
        kind: "archive",
        actions: ["edit_location"],
        automaticDownloads: { mode: "defaults", everySeconds: 21600, maxPerCheck: 10, summary: "Defaults." },
      }),
    );

    await userEvent.click(screen.getByRole("button", { name: /source settings/i }));

    expect(screen.getByRole("button", { name: "Save automatic downloads" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Save different area" })).toBeVisible();
    expect(screen.queryByRole("button", { name: /^Remove / })).not.toBeInTheDocument();
  });
});
