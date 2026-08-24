import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { SupportBundle } from "./support-bundle";

const preview = {
  estimatedBytes: 1536,
  manifest: {
    formatVersion: "loomarr.support-bundle.v1",
    generatedAt: 1,
    selection: { from: 1, to: 2, events: true, processes: true, processOutput: true },
    effectiveFrom: 1,
    effectiveTo: 2,
    loomarr: { version: "v0.9.0" },
    clientVersions: ["web:v0.9.0"],
    entries: [
      { name: "system.json", uncompressedBytes: 128 },
      { name: "events.ndjson", uncompressedBytes: 1408 },
    ],
    counts: {
      events: 12,
      processes: 2,
      processOutputs: 1,
      eventRecorderDrops: 0,
      discardedProcessLines: 0,
      redactions: 3,
    },
    truncationReasons: [],
    uncompressedBytes: 1536,
    finalArchiveBytes: 0,
  },
};

describe("SupportBundle", () => {
  it("reviews selected contents automatically before preparing the download", async () => {
    let sent: unknown;
    server.use(
      http.post("/v1/diagnostics/support-bundle/preview", async ({ request }) => {
        sent = await request.json();
        return HttpResponse.json(preview);
      }),
    );
    render(<SupportBundle correlations={{ channelId: "channel-7" }} />);

    await userEvent.click(screen.getByRole("button", { name: /support bundle/i }));
    expect(await screen.findByRole("region", { name: /support bundle review/i })).toHaveTextContent(
      "About 1.5 KiB",
    );
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /prepare download/i })).toBeEnabled();
    expect(sent).toMatchObject({
      events: true,
      processes: true,
      processOutput: true,
      channelId: "channel-7",
    });
  });

  it("reviews again after the selection changes", async () => {
    server.use(http.post("/v1/diagnostics/support-bundle/preview", () => HttpResponse.json(preview)));
    render(<SupportBundle />);
    await userEvent.click(screen.getByRole("button", { name: /support bundle/i }));
    await waitFor(() => expect(screen.getByRole("button", { name: /prepare download/i })).toBeEnabled());
    await userEvent.click(screen.getByText("Advanced options"));
    await userEvent.click(screen.getByLabelText("Application and player logs"));
    expect(screen.getByRole("button", { name: /prepare download/i })).toBeDisabled();
    await waitFor(() => expect(screen.getByRole("button", { name: /prepare download/i })).toBeEnabled());
  });

  it("keeps Process output coupled to its metadata", async () => {
    render(<SupportBundle />);
    await userEvent.click(screen.getByRole("button", { name: /support bundle/i }));
    await userEvent.click(screen.getByText("Advanced options"));
    await userEvent.click(screen.getByLabelText("Process details"));
    expect(screen.getByLabelText("Process details")).not.toBeChecked();
    expect(screen.getByLabelText("Process output")).not.toBeChecked();
    await userEvent.click(screen.getByLabelText("Process output"));
    expect(screen.getByLabelText("Process details")).toBeChecked();
    expect(screen.getByLabelText("Process output")).toBeChecked();
  });
});
