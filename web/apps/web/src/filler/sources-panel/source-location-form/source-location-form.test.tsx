import { getLocationsSearchMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { fillerSource } from "@/test/fixtures/filler";
import { server } from "@/test/msw/server";
import { SourceLocationForm } from "./source-location-form";

const renderForm = (
  source: ReturnType<typeof fillerSource>,
  actions: { updateSource: ReturnType<typeof vi.fn>; updating?: boolean },
) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <SourceLocationForm
        source={source}
        actions={{ updating: false, ...actions } as Parameters<typeof SourceLocationForm>[0]["actions"]}
      />
    </QueryClientProvider>,
  );

describe("SourceLocationForm", () => {
  it("cannot save a different area until one is chosen", async () => {
    server.use(
      getLocationsSearchMockHandler({
        locations: [{ id: "6167865", label: "Toronto, Canada", country: "CA", place: "6167865" }],
      }),
    );
    const updateSource = vi.fn();
    renderForm(fillerSource({ id: "archive:classic", kind: "archive" }), { updateSource });
    const save = screen.getByRole("button", { name: "Save different area" });
    expect(save).toBeDisabled();

    const location = screen.getByRole("combobox", { name: "Location" });
    await userEvent.clear(location);
    await userEvent.type(location, "Toronto");
    await userEvent.click(await screen.findByRole("option", { name: "Toronto, Canada" }));
    await userEvent.click(save);

    expect(updateSource).toHaveBeenCalledWith("archive:classic", {
      enabled: true,
      geography: { country: "CA" },
    });
  });

  it("clears a source-level area on the server and then locally", async () => {
    const updateSource = vi.fn((_id, _data, callbacks) => callbacks.onSuccess());
    renderForm(
      fillerSource({ id: "archive:classic", kind: "archive", country: "CA", locationSource: "source" }),
      { updateSource },
    );

    await userEvent.click(screen.getByRole("button", { name: "Use my location" }));

    expect(updateSource).toHaveBeenCalledWith(
      "archive:classic",
      { enabled: true, geography: { country: "" } },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    // The draft is emptied once the server confirms; the locally-tracked source flips on refetch.
    expect(screen.getByRole("combobox", { name: "Location" })).toHaveValue("");
  });

  it("drops an unsaved choice without calling the server", async () => {
    const updateSource = vi.fn();
    renderForm(fillerSource({ kind: "archive", country: "CA", locationSource: "installation" }), {
      updateSource,
    });

    await userEvent.click(screen.getByRole("button", { name: "Use my location" }));

    expect(updateSource).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Use my location" })).not.toBeInTheDocument();
  });
});
