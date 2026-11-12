import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { getSettingsListMockHandler, getSetupStatusMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { setting } from "@/test/fixtures/settings";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { PlayoutStep } from "./playout-step";

const ENTRIES: SettingEntry[] = [
  setting({
    key: "server.public_url",
    label: "Loomarr address",
    group: "server",
    kind: "string",
    value: "",
    set: false,
  }),
];

const renderStep = () => {
  server.use(
    getSettingsListMockHandler({ settings: ENTRIES, features: {} }),
    getSetupStatusMockHandler({ checks: [] }),
  );
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <PlayoutStep value="internal" />
    </QueryClientProvider>,
  );
};

describe("PlayoutStep", () => {
  it("shows a failed save, and keeps the address for another try", async () => {
    server.use(
      http.patch("*/v1/settings", () =>
        HttpResponse.json(
          { title: "Could not save settings", status: 500, detail: "The database is busy." },
          { status: 500, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    renderStep();

    await userEvent.type(await screen.findByLabelText("Loomarr address"), "http://loomarr.invalid:8080");
    await userEvent.click(screen.getByRole("button", { name: "Save address" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Could not save settings");
    expect(screen.getByLabelText("Loomarr address")).toHaveValue("http://loomarr.invalid:8080");
    expect(screen.getByRole("button", { name: "Save address" })).toBeInTheDocument();
  });
});
