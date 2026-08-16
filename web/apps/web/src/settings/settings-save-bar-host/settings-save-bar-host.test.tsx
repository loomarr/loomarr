import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsEditsProvider, useSettingsEdits } from "../settings-edits";
import { SettingsSaveBarHost } from "./settings-save-bar-host";

const mocks = vi.hoisted(() => ({ mutate: vi.fn() }));

vi.mock(import("@loomarr/api/endpoints/settings"), async (importOriginal) => {
  const actual = await importOriginal();
  return {
    ...actual,
    useSettingsPatch: (() => ({
      error: null,
      isPending: false,
      mutate: mocks.mutate,
    })) as unknown as typeof actual.useSettingsPatch,
  };
});

const StageEdit = () => {
  const { setEdit } = useSettingsEdits();
  return (
    <button type="button" onClick={() => setEdit("guide.timezone", "America/Chicago")}>
      Stage edit
    </button>
  );
};

const renderHost = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SettingsEditsProvider>
        <StageEdit />
        <SettingsSaveBarHost />
      </SettingsEditsProvider>
    </QueryClientProvider>,
  );

describe("SettingsSaveBarHost", () => {
  beforeEach(() => mocks.mutate.mockReset());

  it("saves the shared edit buffer", async () => {
    renderHost();
    expect(screen.queryByRole("region", { name: "Unsaved changes" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Stage edit" }));
    expect(screen.getByText("1 unsaved change")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(mocks.mutate).toHaveBeenCalledWith({ data: { edits: { "guide.timezone": "America/Chicago" } } });
  });

  it("discards the shared edit buffer", async () => {
    renderHost();
    await userEvent.click(screen.getByRole("button", { name: "Stage edit" }));
    await userEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(screen.queryByRole("region", { name: "Unsaved changes" })).not.toBeInTheDocument();
  });
});
