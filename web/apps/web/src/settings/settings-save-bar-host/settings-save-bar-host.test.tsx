import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { SettingsEditsProvider, useSettingsEdits } from "../settings-edits";
import { SettingsSaveBarHost } from "./settings-save-bar-host";

const mocks = vi.hoisted(() => ({
  mutate: vi.fn(),
  onError: undefined as undefined | ((error: unknown) => void),
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
    useSettingsPatch: ((options: {
      mutation?: { onSuccess?: typeof mocks.onSuccess; onError?: (error: unknown) => void };
    }) => {
      mocks.onSuccess = options.mutation?.onSuccess;
      mocks.onError = options.mutation?.onError;
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
  const { results, setEdit } = useSettingsEdits();
  return (
    <>
      <button
        type="button"
        onClick={() => {
          setEdit("filler.home_country", "US");
          setEdit("filler.home_place", "5128581");
        }}
      >
        Stage two edits
      </button>
      <output aria-label="Save results">
        {results.map((result) => `${result.key}:${result.status}`).join(",")}
      </output>
    </>
  );
};

const renderHost = ({
  twoEdits = false,
  client = new QueryClient(),
}: {
  twoEdits?: boolean;
  client?: QueryClient;
} = {}) =>
  render(
    <QueryClientProvider client={client}>
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
            { key: "filler.home_place", status: "pinned" },
          ],
        },
      });
    });

    expect(screen.getByText("1 unsaved change")).toBeInTheDocument();
    expect(screen.getByLabelText("Save results")).toHaveTextContent("filler.home_place:pinned");
  });

  it("refetches every settings-derived read on save, including AI status", async () => {
    const client = new QueryClient();
    await client.prefetchQuery({ queryKey: ["/v1/system/llm"], queryFn: () => "status" });
    await client.prefetchQuery({ queryKey: ["/v1/settings"], queryFn: () => "list" });
    renderHost({ client });
    await userEvent.click(screen.getByRole("button", { name: "Stage edit" }));
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await act(async () => {
      await mocks.onSuccess?.({
        status: 200,
        data: { results: [{ key: "guide.timezone", status: "saved" }] },
      });
    });

    expect(client.getQueryState(["/v1/system/llm"])?.isInvalidated).toBe(true);
    expect(client.getQueryState(["/v1/settings"])?.isInvalidated).toBe(true);
  });

  it("confirms a save with a toast, and says so when the save fails", async () => {
    renderHost();
    await userEvent.click(screen.getByRole("button", { name: "Stage edit" }));
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await act(async () => {
      await mocks.onSuccess?.({
        status: 200,
        data: { results: [{ key: "guide.timezone", status: "saved" }] },
      });
    });
    expect(toast.success).toHaveBeenCalledWith("Change saved");

    mocks.onError?.(new Error("offline"));
    expect(toast.error).toHaveBeenCalledWith("Couldn't save your changes", { description: "offline" });
  });
});
