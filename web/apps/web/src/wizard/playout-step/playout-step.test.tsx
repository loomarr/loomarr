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

const TUNARR: SettingEntry[] = [
  setting({
    key: "tunarr.url",
    label: "Tunarr address",
    group: "connections.tunarr",
    kind: "string",
    value: "",
    set: false,
  }),
  setting({
    // A second Tunarr field; the key is a stand-in, the label is what the operator sees.
    key: SettingKey.TunarrTranscodeConfigID,
    label: "Tunarr token",
    group: "connections.tunarr",
    kind: "string",
    value: "",
    set: false,
  }),
];

describe("PlayoutStep", () => {
  it("saves the Loomarr address alone", async () => {
    const sent: unknown[] = [];
    server.use(
      getSettingsPatchMockHandler(async ({ request }) => {
        sent.push(await request.json());
        return { results: [{ key: "server.public_url", status: "saved" }] };
      }),
    );
    renderStep();

    await userEvent.type(await screen.findByLabelText("Loomarr address"), "http://loomarr:8080");
    await userEvent.click(screen.getByRole("button", { name: "Save address" }));

    await waitFor(() => expect(sent).toEqual([{ edits: { "server.public_url": "http://loomarr:8080" } }]));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Save address" })).not.toBeInTheDocument(),
    );
  });

  it("saves Tunarr's own fields, keeping the one the server refused", async () => {
    const sent: unknown[] = [];
    server.use(
      getSettingsListMockHandler({ settings: TUNARR, features: {} }),
      getSetupStatusMockHandler({ checks: [] }),
      getSettingsPatchMockHandler(async ({ request }) => {
        sent.push(await request.json());
        return {
          results: [
            { key: "tunarr.url", status: "saved" },
            {
              key: SettingKey.TunarrTranscodeConfigID,
              status: "invalid",
              problem: "Tunarr refused that token.",
            },
          ],
        };
      }),
    );
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <PlayoutStep value="tunarr" />
      </QueryClientProvider>,
    );

    await userEvent.type(await screen.findByLabelText("Tunarr address"), "http://tunarr:8000");
    await userEvent.type(screen.getByLabelText("Tunarr token"), "nope");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(sent).toEqual([
        { edits: { "tunarr.url": "http://tunarr:8000", [SettingKey.TunarrTranscodeConfigID]: "nope" } },
      ]),
    );
    expect(await screen.findByText("Tunarr refused that token.")).toBeInTheDocument();
    expect(screen.getByLabelText("Tunarr token")).toHaveValue("nope");
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
  });

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
