import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { SettingKey } from "@loomarr/api/models/settingKey";
import {
  getSettingsListMockHandler,
  getSettingsPatchMockHandler,
  getSetupStatusMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { setting } from "@/test/fixtures/settings";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { ChecklistStep } from "./checklist-step";

const ENTRIES: SettingEntry[] = [
  setting({
    key: SettingKey.TMDBAPIKey,
    label: "TMDB key",
    group: "connections.tmdb",
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
      <ChecklistStep openId="tmdb" only={["tmdb"]} />
    </QueryClientProvider>,
  );
};

describe("ChecklistStep", () => {
  it("keeps a value the server rejected in the form, and does not test the old settings", async () => {
    let tests = 0;
    server.use(
      getSettingsPatchMockHandler({
        results: [{ key: SettingKey.TMDBAPIKey, status: "invalid", problem: "That key was not accepted." }],
      }),
      http.post("*/v1/setup/test", () => {
        tests += 1;
        return HttpResponse.json({ ok: true });
      }),
    );
    renderStep();

    await userEvent.type(await screen.findByLabelText("TMDB key"), "not-a-key");
    await userEvent.click(screen.getByRole("button", { name: "Test connection" }));

    expect(await screen.findByText("That key was not accepted.")).toBeInTheDocument();
    expect(screen.getByLabelText("TMDB key")).toHaveValue("not-a-key");
    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();
    expect(tests).toBe(0);
  });

  it("clears the edit once the server saved it, then tests what was saved", async () => {
    let tests = 0;
    server.use(
      getSettingsPatchMockHandler({ results: [{ key: SettingKey.TMDBAPIKey, status: "saved" }] }),
      http.post("*/v1/setup/test", () => {
        tests += 1;
        return HttpResponse.json({ ok: true });
      }),
    );
    renderStep();

    await userEvent.type(await screen.findByLabelText("TMDB key"), "good-key");
    await userEvent.click(screen.getByRole("button", { name: "Test connection" }));

    await waitFor(() => expect(tests).toBe(1));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument(),
    );
  });
});
