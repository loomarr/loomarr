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
  setting({
    // A second field in the block; the key is a stand-in, the label is what the operator sees.
    key: SettingKey.TunarrTranscodeConfigID,
    label: "TMDB region",
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
  it("saves only the edited keys in one PATCH", async () => {
    const sent: unknown[] = [];
    server.use(
      getSettingsPatchMockHandler(async ({ request }) => {
        sent.push(await request.json());
        return { results: [{ key: SettingKey.TMDBAPIKey, status: "saved" }] };
      }),
    );
    renderStep();

    await userEvent.type(await screen.findByLabelText("TMDB key"), "good-key");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(sent).toEqual([{ edits: { [SettingKey.TMDBAPIKey]: "good-key" } }]));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument(),
    );
  });

  it("drops the keys the server saved and keeps the ones it refused", async () => {
    server.use(
      getSettingsPatchMockHandler({
        results: [
          { key: SettingKey.TMDBAPIKey, status: "saved" },
          { key: SettingKey.TunarrTranscodeConfigID, status: "invalid", problem: "Not a region." },
        ],
      }),
    );
    renderStep();

    await userEvent.type(await screen.findByLabelText("TMDB key"), "good-key");
    await userEvent.type(screen.getByLabelText("TMDB region"), "zz");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(await screen.findByText("Not a region.")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByLabelText("TMDB key")).toHaveValue(""));
    expect(screen.getByLabelText("TMDB region")).toHaveValue("zz");
    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();
  });

  it("discards the staged edits without sending anything", async () => {
    let patches = 0;
    server.use(
      getSettingsPatchMockHandler(() => {
        patches += 1;
        return { results: [] };
      }),
    );
    renderStep();

    await userEvent.type(await screen.findByLabelText("TMDB key"), "typo");
    await userEvent.click(screen.getByRole("button", { name: "Discard" }));

    expect(screen.getByLabelText("TMDB key")).toHaveValue("");
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
    expect(patches).toBe(0);
  });

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
