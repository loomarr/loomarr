import { SizeConfidence } from "@loomarr/api/models/sizeConfidence";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { BulkApproveDialog } from "./bulk-approve-dialog";
import type { BulkApproveDialogProps, ChannelRequestConsequence } from "./bulk-approve-dialog.type";

const GiB = 1024 ** 3;

const request = (over: Partial<ChannelRequestConsequence> = {}): ChannelRequestConsequence => ({
  id: "cr-1",
  title: "Rainy afternoon serials",
  titleCount: 2,
  toDownload: 1,
  ceiling: "TV-14",
  sizes: [{ sizeBytes: 2.1 * GiB, sizeConfidence: SizeConfidence.exact }, {}],
  channel: { kind: "create", name: "Rainy afternoon serials" },
  ...over,
});

const renderDialog = (props: Partial<BulkApproveDialogProps> = {}) => {
  const handlers = { onCancel: vi.fn(), onConfirm: vi.fn(), onRetry: vi.fn(), onDone: vi.fn() };
  render(
    <BulkApproveDialog
      open
      consequences={{ group: "channels", items: [request()] }}
      {...handlers}
      {...props}
    />,
  );
  return handlers;
};

describe("BulkApproveDialog — the consequence summary", () => {
  it("states the scope, each item's effect, and the selection's size honestly", () => {
    renderDialog();

    expect(screen.getByRole("dialog", { name: "Approve 1 channel request?" })).toBeInTheDocument();
    expect(screen.getByText("Channel requests")).toBeInTheDocument();
    expect(screen.getByText("2 titles · 1 to download · up to TV-14")).toBeInTheDocument();
    expect(screen.getAllByText("Size: 2.1 GB for 1 of 2 titles · 1 unknown")).toHaveLength(2);
    expect(screen.getByText('Becomes a new channel named "Rainy afternoon serials"')).toBeInTheDocument();
    expect(screen.getByText("1 title will start downloading.")).toBeInTheDocument();
  });

  it("states an all-known exact sum plainly, and an estimate as one", () => {
    renderDialog({
      consequences: {
        group: "channels",
        items: [
          request({ sizes: [{ sizeBytes: 2.1 * GiB, sizeConfidence: SizeConfidence.exact }] }),
          request({ id: "cr-2", sizes: [{ sizeBytes: GiB, sizeConfidence: SizeConfidence.estimated }] }),
        ],
      },
    });

    expect(screen.getByText("Size: 2.1 GB")).toBeInTheDocument();
    expect(screen.getAllByText("Size: ~1.0 GB (estimated)")).toHaveLength(1);
    expect(screen.getByText("Size: ~3.1 GB (estimated)")).toBeInTheDocument();
  });

  it("never renders 0 B or drops the line when nothing is known", () => {
    renderDialog({ consequences: { group: "channels", items: [request({ sizes: [{}, {}] })] } });

    expect(screen.getAllByText("Size: estimate unavailable")).toHaveLength(2);
    expect(screen.queryByText(/0 B/)).not.toBeInTheDocument();
  });

  it("drops the ceiling clause when the request has none", () => {
    const { ceiling: _, ...noCeiling } = request();
    renderDialog({ consequences: { group: "channels", items: [noCeiling] } });

    expect(screen.getByText("2 titles · 1 to download")).toBeInTheDocument();
  });

  it("says it updates the channel a revised request already has", () => {
    renderDialog({
      consequences: {
        group: "channels",
        items: [request({ channel: { kind: "update", name: "Grey Days" } })],
      },
    });

    expect(screen.getByText('Updates the channel "Grey Days"')).toBeInTheDocument();
  });

  it("marks filler as an estimate with size and channel unavailable", () => {
    renderDialog({
      consequences: {
        group: "filler",
        items: [{ id: "fp-1", title: "Station-ident bumpers", estimateClips: 18, sources: 2 }],
      },
    });

    expect(screen.getByRole("dialog", { name: "Approve 1 filler download?" })).toBeInTheDocument();
    expect(
      screen.getByText("~18 clips from 2 sources (estimate — may yield fewer once fetched)"),
    ).toBeInTheDocument();
    expect(screen.getByText("Channel: unavailable")).toBeInTheDocument();
    expect(screen.getAllByText("Size: estimate unavailable")).toHaveLength(2);
    expect(screen.getByText("~18 clips will start downloading, where still available.")).toBeInTheDocument();
    expect(screen.queryByText(/General catalog variety/)).not.toBeInTheDocument();
  });

  it("does not forecast ~0 clips for an unmeasured pull", () => {
    renderDialog({
      consequences: {
        group: "filler",
        items: [{ id: "fp-1", title: "Bumpers", estimateClips: 0, sources: 1 }],
      },
    });

    expect(screen.getByText("From 1 source (no clip estimate yet)")).toBeInTheDocument();
    expect(screen.queryByText(/~0 clips/)).not.toBeInTheDocument();
  });

  it("confirms with the Approve button and cancels with Escape", async () => {
    const { onConfirm, onCancel } = renderDialog();

    await userEvent.click(screen.getByRole("button", { name: "Approve 1" }));
    expect(onConfirm).toHaveBeenCalledOnce();

    await userEvent.keyboard("{Escape}");
    expect(onCancel).toHaveBeenCalledOnce();
  });

  // Closing mid-call would orphan a request whose result nobody then sees.
  it("cannot be dismissed while the call is in flight", async () => {
    const { onCancel } = renderDialog({ pending: true });

    expect(screen.getByRole("button", { name: "Approving…" })).toBeDisabled();
    await userEvent.keyboard("{Escape}");
    expect(onCancel).not.toHaveBeenCalled();
  });
});

describe("BulkApproveDialog — the result", () => {
  it("lists each outcome with its exact count, and retries one or every failure", async () => {
    const { onRetry, onDone } = renderDialog({
      consequences: {
        group: "channels",
        items: [request(), request({ id: "cr-2", title: "Overnight documentary block" })],
      },
      results: [
        { id: "cr-1", ok: true, enqueued: 3 },
        { id: "cr-2", ok: false, error: "This suggestion has already been approved or dismissed." },
      ],
    });

    expect(screen.getByRole("dialog", { name: "Approved 1 of 2" })).toBeInTheDocument();
    expect(
      screen.getByText('3 titles enqueued · channel "Rainy afternoon serials" created'),
    ).toBeInTheDocument();
    expect(screen.getByText("This suggestion has already been approved or dismissed.")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Retry Overnight documentary block" }));
    expect(onRetry).toHaveBeenLastCalledWith(["cr-2"]);
    await userEvent.click(screen.getByRole("button", { name: "Retry 1 failed" }));
    expect(onRetry).toHaveBeenLastCalledWith(["cr-2"]);

    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(onDone).toHaveBeenCalledOnce();
  });

  it("reads 'N clips queued' for an approved pull — exact, no estimate language", () => {
    renderDialog({
      consequences: {
        group: "filler",
        items: [{ id: "fp-1", title: "Bumpers", estimateClips: 18, sources: 2 }],
      },
      results: [{ id: "fp-1", ok: true, enqueued: 12 }],
    });

    expect(screen.getByText("12 clips queued")).toBeInTheDocument();
  });

  it("says 'updated' for a channel the approval replaced", () => {
    renderDialog({
      consequences: {
        group: "channels",
        items: [request({ channel: { kind: "update", name: "Grey Days" } })],
      },
      results: [{ id: "cr-1", ok: true, enqueued: 1 }],
    });

    expect(screen.getByText('1 title enqueued · channel "Grey Days" updated')).toBeInTheDocument();
  });

  it("closing the result with Escape is Done", async () => {
    const { onDone, onCancel } = renderDialog({ results: [{ id: "cr-1", ok: true, enqueued: 1 }] });

    await userEvent.keyboard("{Escape}");
    expect(onDone).toHaveBeenCalledOnce();
    expect(onCancel).not.toHaveBeenCalled();
  });
});
