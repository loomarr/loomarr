import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
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
  it("uses a plain-language one-action troubleshooting download", async () => {
    let sent: unknown;
    let downloaded = false;
    server.use(
      http.post("/v1/diagnostics/support-bundle/preview", async ({ request }) => {
        sent = await request.json();
        return HttpResponse.json(preview);
      }),
      http.post("/v1/diagnostics/support-bundle", () => {
        downloaded = true;
        return new HttpResponse(new Blob(["zip"]), {
          headers: { "Content-Disposition": 'attachment; filename="loomarr-report.zip"' },
        });
      }),
    );
    vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:report");
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
    render(<SupportBundle correlations={{ channelId: "channel-7" }} />);

    await userEvent.click(screen.getByRole("button", { name: "Download troubleshooting report" }));
    expect(await screen.findByRole("region", { name: "Troubleshooting report summary" })).toHaveTextContent(
      "About 1.5 KiB",
    );
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Download report" })).toBeEnabled();
    expect(screen.queryByText(/1 · Review|2 · Download/)).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /prepare download|back to review/i }),
    ).not.toBeInTheDocument();
    expect(sent).toMatchObject({
      events: true,
      processes: true,
      processOutput: true,
      channelId: "channel-7",
    });
    await userEvent.click(screen.getByRole("button", { name: "Download report" }));
    await waitFor(() => expect(downloaded).toBe(true));
  });

  it("reviews again after the selection changes", async () => {
    server.use(http.post("/v1/diagnostics/support-bundle/preview", () => HttpResponse.json(preview)));
    render(<SupportBundle />);
    await userEvent.click(screen.getByRole("button", { name: "Download troubleshooting report" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Download report" })).toBeEnabled());
    await userEvent.click(screen.getByText("Customize report"));
    await userEvent.click(screen.getByLabelText("Application and player logs"));
    expect(screen.getByRole("button", { name: "Download report" })).toBeDisabled();
    await waitFor(() => expect(screen.getByRole("button", { name: "Download report" })).toBeEnabled());
  });

  it("keeps Process output coupled to its metadata", async () => {
    server.use(http.post("/v1/diagnostics/support-bundle/preview", () => HttpResponse.json(preview)));
    render(<SupportBundle />);
    await userEvent.click(screen.getByRole("button", { name: "Download troubleshooting report" }));
    await userEvent.click(screen.getByText("Customize report"));
    await userEvent.click(screen.getByLabelText("Process details"));
    expect(screen.getByLabelText("Process details")).not.toBeChecked();
    expect(screen.getByLabelText("Process output")).not.toBeChecked();
    await userEvent.click(screen.getByLabelText("Process output"));
    expect(screen.getByLabelText("Process details")).toBeChecked();
    expect(screen.getByLabelText("Process output")).toBeChecked();
  });
});
