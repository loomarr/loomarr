// @vitest-environment jsdom

import { SizeConfidence } from "@loomarr/core/requests";
import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode, useState } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  ApprovalBulkGroup,
  type ApprovalBulkRow,
  ApprovalConsequenceDialog,
  type ApprovalConsequenceDialogProps,
  type ChannelRequestConsequence,
} from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Node = {
  click(): void;
  getAttribute(name: string): string | null;
  textContent: string | null;
};
type Doc = {
  body: {
    append(node: unknown): void;
    textContent: string | null;
    querySelectorAll(s: string): ArrayLike<Node>;
  };
  createElement(tag: string): Parameters<typeof createRoot>[0] & { remove(): void };
  dispatchEvent(event: unknown): boolean;
};
const doc = (globalThis as unknown as { document: Doc }).document;

// The modal portals into <body>, so every query reads the whole document.
const mount = async (node: ReactNode) => {
  const container = doc.createElement("div");
  doc.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);
  });
  return {
    text: () => doc.body.textContent ?? "",
    unmount: () =>
      act(() => {
        root.unmount();
        container.remove();
      }),
  };
};
// A failed test skips its own unmount; never let its DOM leak into the next one.
afterEach(() => {
  for (const node of Array.from(doc.body.querySelectorAll(":scope > *")))
    (node as unknown as { remove(): void }).remove();
});

const byRole = (role: string) => Array.from(doc.body.querySelectorAll(`[role="${role}"]`));
const named = (role: string, name: string) => {
  const found = byRole(role).find(
    (n) => (n.getAttribute("aria-label") ?? n.textContent?.trim()) === name || n.textContent?.trim() === name,
  );
  if (!found) throw new Error(`missing ${role}: ${name}`);
  return found;
};
const click = (node: Node) => act(async () => node.click());

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

const dialog = async (props: Partial<ApprovalConsequenceDialogProps> = {}) => {
  const handlers = { onCancel: vi.fn(), onConfirm: vi.fn(), onDone: vi.fn(), onRetry: vi.fn() };
  const view = await mount(
    <ApprovalConsequenceDialog
      consequences={{ group: "channels", items: [request()] }}
      visible
      {...handlers}
      {...props}
    />,
  );
  return { ...handlers, ...view };
};

describe("ApprovalConsequenceDialog — the summary", () => {
  it("states the scope, each item's effect, and the selection's size honestly", async () => {
    const view = await dialog();
    const text = view.text();
    expect(text).toContain("Approve 1 channel request?");
    expect(text).toContain("Scope: this approves only the Channel requests group.");
    expect(text).toContain("2 titles · 1 to download · up to TV-14");
    expect(text.match(/Size: 2\.1 GB for 1 of 2 titles · 1 unknown/g)).toHaveLength(2);
    expect(text).toContain('Becomes a new channel named "Rainy afternoon serials"');
    expect(text).toContain("1 title will start downloading.");
    await view.unmount();
  });

  it("states an all-known sum plainly, and an estimate as one", async () => {
    const view = await dialog({
      consequences: {
        group: "channels",
        items: [
          request({ sizes: [{ sizeBytes: 2.1 * GiB, sizeConfidence: SizeConfidence.exact }] }),
          request({ id: "cr-2", sizes: [{ sizeBytes: GiB, sizeConfidence: SizeConfidence.estimated }] }),
        ],
      },
    });
    expect(view.text()).toContain("Size: 2.1 GB");
    expect(view.text()).toContain("Size: ~1.0 GB (estimated)");
    expect(view.text()).toContain("Size: ~3.1 GB (estimated)");
    await view.unmount();
  });

  it("never renders 0 B or drops the line when nothing is known", async () => {
    const view = await dialog({ consequences: { group: "channels", items: [request({ sizes: [{}, {}] })] } });
    expect(view.text().match(/Size: estimate unavailable/g)).toHaveLength(2);
    expect(view.text()).not.toContain("0 B");
    await view.unmount();
  });

  it("drops the ceiling clause when the request has none", async () => {
    const { ceiling: _, ...noCeiling } = request();
    const view = await dialog({ consequences: { group: "channels", items: [noCeiling] } });
    expect(view.text()).toContain("2 titles · 1 to download");
    expect(view.text()).not.toContain("up to");
    await view.unmount();
  });

  it("says it updates the channel a revised request already has", async () => {
    const view = await dialog({
      consequences: {
        group: "channels",
        items: [request({ channel: { kind: "update", name: "Grey Days" } })],
      },
    });
    expect(view.text()).toContain('Updates the channel "Grey Days"');
    expect(view.text()).not.toContain("Becomes a new channel");
    await view.unmount();
  });

  it("marks filler as an estimate with size and channel unavailable", async () => {
    const view = await dialog({
      consequences: {
        group: "filler",
        items: [{ id: "fp-1", title: "Station-ident bumpers", estimateClips: 18, sources: 2 }],
      },
    });
    const text = view.text();
    expect(text).toContain("Approve 1 filler download?");
    expect(text).toContain("~18 clips from 2 sources (estimate — may yield fewer once fetched)");
    expect(text).toContain("Channel: unavailable");
    expect(text.match(/Size: estimate unavailable/g)).toHaveLength(2);
    expect(text).toContain("~18 clips will start downloading, where still available.");
    expect(text).not.toContain("General catalog variety");
    await view.unmount();
  });

  it("does not forecast ~0 clips for an unmeasured pull", async () => {
    const view = await dialog({
      consequences: {
        group: "filler",
        items: [{ id: "fp-1", title: "Bumpers", estimateClips: 0, sources: 1 }],
      },
    });
    expect(view.text()).toContain("From 1 source (no clip estimate yet)");
    expect(view.text()).not.toContain("~0 clips");
    await view.unmount();
  });

  it("confirms with Approve and cancels with Cancel", async () => {
    const view = await dialog();
    await click(named("button", "Approve 1"));
    expect(view.onConfirm).toHaveBeenCalledOnce();
    await click(named("button", "Cancel"));
    expect(view.onCancel).toHaveBeenCalledOnce();
    await view.unmount();
  });

  it("states a whole-call failure where the operator is looking", async () => {
    const view = await dialog({ error: "Couldn't reach Loomarr." });
    expect(named("alert", "Couldn't reach Loomarr.")).toBeTruthy();
    await view.unmount();
  });

  it("holds its buttons while the call is in flight", async () => {
    const view = await dialog({ pending: true });
    await click(named("button", "Approving…"));
    await click(named("button", "Cancel"));
    expect(view.onConfirm).not.toHaveBeenCalled();
    expect(view.onCancel).not.toHaveBeenCalled();
    await view.unmount();
  });
});

describe("ApprovalConsequenceDialog — the result", () => {
  it("lists each outcome, and retries only the failures the server marked retryable", async () => {
    const view = await dialog({
      consequences: {
        group: "channels",
        items: [
          request(),
          request({ id: "cr-2", title: "Overnight documentary block" }),
          request({ id: "cr-3", title: "Saturday cartoons" }),
          request({ id: "cr-4", title: "Late-night jazz" }),
        ],
      },
      results: [
        { id: "cr-1", ok: true, enqueued: 3 },
        { id: "cr-2", ok: false, error: "This suggestion has already been approved or dismissed." },
        { id: "cr-3", ok: false, error: "Couldn't save the approval.", retryable: true },
        { id: "cr-4", ok: false, error: "Couldn't save the approval.", retryable: true },
      ],
    });
    const text = view.text();
    expect(text).toContain("Approved 1 of 4");
    expect(text).toContain('3 titles enqueued · channel "Rainy afternoon serials" created');
    expect(text).toContain("This suggestion has already been approved or dismissed.");
    expect(named("img", "Approved")).toBeTruthy();
    expect(byRole("img").filter((n) => n.getAttribute("aria-label") === "Not approved")).toHaveLength(3);

    // A permanent failure states its reason with nothing to press: a retry could only repeat it.
    expect(() => named("button", "Retry Overnight documentary block")).toThrow();
    await click(named("button", "Retry Saturday cartoons"));
    expect(view.onRetry).toHaveBeenLastCalledWith(["cr-3"]);
    await click(named("button", "Retry 2 failed"));
    expect(view.onRetry).toHaveBeenLastCalledWith(["cr-3", "cr-4"]);
    await click(named("button", "Done"));
    expect(view.onDone).toHaveBeenCalledOnce();
    await view.unmount();
  });

  it("offers no retry at all when every failure is permanent", async () => {
    const view = await dialog({
      consequences: {
        group: "channels",
        items: [request(), request({ id: "cr-2", title: "Overnight documentary block" })],
      },
      results: [
        { id: "cr-1", ok: true, enqueued: 3 },
        { id: "cr-2", ok: false, error: "This suggestion has already been approved or dismissed." },
      ],
    });
    expect(view.text()).toContain("This suggestion has already been approved or dismissed.");
    expect(byRole("button").filter((n) => n.textContent?.startsWith("Retry"))).toHaveLength(0);
    expect(view.text()).not.toMatch(/Retry \d+ failed/);
    await view.unmount();
  });

  it("reads 'N clips queued' for an approved pull — exact, no estimate language", async () => {
    const view = await dialog({
      consequences: {
        group: "filler",
        items: [{ id: "fp-1", title: "Bumpers", estimateClips: 18, sources: 2 }],
      },
      results: [{ id: "fp-1", ok: true, enqueued: 12 }],
    });
    expect(view.text()).toContain("12 clips queued");
    await view.unmount();
  });

  it("says 'updated' for a channel the approval replaced", async () => {
    const view = await dialog({
      consequences: {
        group: "channels",
        items: [request({ channel: { kind: "update", name: "Grey Days" } })],
      },
      results: [{ id: "cr-1", ok: true, enqueued: 1 }],
    });
    expect(view.text()).toContain('1 title enqueued · channel "Grey Days" updated');
    await view.unmount();
  });
});

const row = (id: string, label: string, excluded = false): ApprovalBulkRow => ({
  id,
  label,
  excluded,
  content: <span>{label}</span>,
});

const Group = ({ rows, onApprove = () => {} }: { rows: ApprovalBulkRow[]; onApprove?: () => void }) => {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  return (
    <ApprovalBulkGroup
      onApprove={onApprove}
      onSelectedChange={setSelected}
      rows={rows}
      selected={selected}
      title="Channel requests"
    />
  );
};

describe("ApprovalBulkGroup", () => {
  it("is a named group whose Approve counts the selection and is disabled at zero", async () => {
    const onApprove = vi.fn();
    const view = await mount(<Group onApprove={onApprove} rows={[row("a", "Noir"), row("b", "Westerns")]} />);
    expect(named("group", "Channel requests")).toBeTruthy();
    expect(view.text()).toContain("None selected");
    await click(named("button", "Approve"));
    expect(onApprove).not.toHaveBeenCalled();

    await click(named("checkbox", "Select Noir for bulk approve"));
    expect(view.text()).toContain("1 selected");
    await click(named("button", "Approve 1"));
    expect(onApprove).toHaveBeenCalledOnce();
    await view.unmount();
  });

  it("selects and clears every bulkable row from select-all", async () => {
    const view = await mount(<Group rows={[row("a", "Noir"), row("b", "Westerns")]} />);
    await click(named("checkbox", "Select all Channel requests"));
    expect(view.text()).toContain("2 selected");
    await click(named("checkbox", "Select all Channel requests"));
    expect(view.text()).toContain("None selected");
    await view.unmount();
  });

  it("gives an excluded row no checkbox and says why, and select-all skips it", async () => {
    const view = await mount(
      <Group rows={[row("a", "Noir"), row("b", "Westerns"), row("c", "Edited one", true)]} />,
    );
    expect(() => named("checkbox", "Select Edited one for bulk approve")).toThrow();
    expect(view.text()).toContain("Excluded from bulk — approve individually so the edit is kept.");
    await click(named("checkbox", "Select all Channel requests"));
    expect(named("button", "Approve 2")).toBeTruthy();
    await view.unmount();
  });

  it("offers no select-all over a single bulkable row", async () => {
    const view = await mount(<Group rows={[row("a", "Noir"), row("c", "Edited one", true)]} />);
    expect(() => named("checkbox", "Select all Channel requests")).toThrow();
    expect(named("checkbox", "Select Noir for bulk approve")).toBeTruthy();
    await view.unmount();
  });
});
