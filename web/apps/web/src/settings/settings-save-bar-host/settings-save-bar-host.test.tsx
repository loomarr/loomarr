import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsEditsProvider, useSettingsEdits } from "../settings-edits";
import { SettingsSaveBarHost } from "./settings-save-bar-host";

const mocks = vi.hoisted(() => ({
  mutate: vi.fn(),
  onSuccess: undefined as
    | undefined
    | ((response: {
        status: number;
        data: { results: Array<{ key: string; status: "saved" | "invalid" | "pinned" }> };
      }) => Promise<void>),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock(import("@loomarr/api/endpoints/settings"), async (importOriginal) => {
  const actual = await importOriginal();
  return {
    ...actual,
    useSettingsPatch: ((options: { mutation?: { onSuccess?: typeof mocks.onSuccess } }) => {
      mocks.onSuccess = options.mutation?.onSuccess;
      return {
        error: null,
        isPending: false,
        mutate: mocks.mutate,
      };
    }) as unknown as typeof actual.useSettingsPatch,
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

const StageTwoEdits = () => {
  const { setEdit } = useSettingsEdits();
  return (
    <button
      type="button"
      onClick={() => {
        setEdit("filler.home_country", "US");
        setEdit("filler.home_market", "New York City");
      }}
    >
      Stage two edits
    </button>
  );
};

const renderHost = ({ twoEdits = false }: { twoEdits?: boolean } = {}) =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SettingsEditsProvider>
        {twoEdits ? <StageTwoEdits /> : <StageEdit />}
        <SettingsSaveBarHost />
      </SettingsEditsProvider>
    </QueryClientProvider>,
  );

describe("SettingsSaveBarHost", () => {
  beforeEach(() => {
    mocks.mutate.mockReset();
    mocks.onSuccess = undefined;
  });

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

  it("clears only saved edits when a partial save rejects another key", async () => {
    renderHost({ twoEdits: true });
    await userEvent.click(screen.getByRole("button", { name: "Stage two edits" }));
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(mocks.onSuccess).toBeDefined();
    await act(async () => {
      await mocks.onSuccess?.({
        status: 200,
        data: {
          results: [
            { key: "filler.home_country", status: "saved" },
            { key: "filler.home_market", status: "pinned" },
          ],
        },
      });
    });

    expect(screen.getByText("1 unsaved change")).toBeInTheDocument();
  });
});
