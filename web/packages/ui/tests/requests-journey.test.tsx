// @vitest-environment jsdom

import type { RequestsController, RequestsSnapshot } from "@loomarr/core/requests";
import { LoomarrProvider } from "@loomarr/design-system";
import { requestFixtures, requestsSnapshot } from "@loomarr/fixtures";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestsJourney } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// The props of every pull-to-refresh control the journey draws, newest last.
const refreshControls = vi.hoisted(
  () => [] as { onRefresh?: () => void; refreshing?: boolean; children?: ReactNode }[],
);
vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return {
    ...actual,
    RefreshControl: (props: (typeof refreshControls)[number]) => {
      refreshControls.push(props);
      return props.children;
    },
  };
});

type Node = {
  click(): void;
  getAttribute(name: string): string | null;
  querySelectorAll(selector: string): ArrayLike<unknown>;
  textContent: string | null;
};
type Doc = {
  body: {
    append(node: unknown): void;
    textContent: string | null;
    querySelectorAll(s: string): ArrayLike<Node>;
  };
  createElement(tag: string): Parameters<typeof createRoot>[0] & { remove(): void };
};
const doc = (globalThis as unknown as { document: Doc }).document;

const { journeys } = requestFixtures;
const noop = vi.fn();

// A controller whose snapshot a test can replace, as a refetch would.
const fakeController = (initial: RequestsSnapshot) => {
  let snapshot = initial;
  const listeners = new Set<() => void>();
  const controller: RequestsController = {
    approve: vi.fn(),
    approveFillerPull: vi.fn(),
    approveFillerPulls: vi.fn(),
    approveMany: vi.fn(),
    deny: vi.fn(),
    dismissFillerPull: vi.fn(),
    dismissNotice: vi.fn(),
    dispose: vi.fn(),
    getSnapshot: () => snapshot,
    hideIdea: vi.fn(),
    refresh: vi.fn(async () => undefined),
    refreshIdeas: vi.fn(async () => undefined),
    requestIdea: vi.fn(),
    submitBrief: vi.fn(),
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    undoHideIdea: vi.fn(),
  };
  return {
    controller,
    set: (next: RequestsSnapshot) => {
      snapshot = next;
      for (const listener of listeners) listener();
    },
  };
};

const only = (...ids: (keyof typeof journeys)[]) =>
  requestsSnapshot().entries.filter((entry) => ids.some((id) => journeys[id].jobId === entry.journey.jobId));

const mounted: { unmount: () => unknown }[] = [];
const guideChannels = [{ id: "channel-rainy", name: "Rainy Day Detectives", number: 14 }];
const mount = async (controller: RequestsController, channels = guideChannels) => {
  const container = doc.createElement("div");
  doc.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(
      <LoomarrProvider theme="dark">
        <RequestsJourney
          channels={channels}
          controller={controller}
          nowMs={Date.UTC(2026, 9, 1)}
          onOpenChannel={noop}
        />
      </LoomarrProvider>,
    );
  });
  const handle = {
    unmount: () =>
      act(() => {
        root.unmount();
        container.remove();
      }),
  };
  mounted.push(handle);
  return handle;
};
afterEach(async () => {
  for (const handle of mounted.splice(0)) await handle.unmount();
  refreshControls.length = 0;
  noop.mockClear();
});

const all = (selector: string) => Array.from(doc.body.querySelectorAll(selector));
const byLabel = (label: string) => {
  const found = all(`[aria-label="${label}"]`)[0];
  if (!found) throw new Error(`missing: ${label}`);
  return found;
};
const click = (node: Node) => act(async () => node.click());
const selectedSection = () =>
  all('[role="tab"][aria-selected="true"]').map((tab) => tab.getAttribute("aria-label"));

describe("the section Requests opens on", () => {
  it.each([
    ["Needs you", only("failed", "generating", "live"), "Needs you, 1"],
    ["In progress", only("generating", "live"), "In progress, 1"],
    ["Done", only("live", "denied"), "Done, 2"],
  ] as const)("is the first with something in it: %s", async (_name, entries, selected) => {
    await mount(fakeController(requestsSnapshot({ entries })).controller);
    expect(selectedSection()).toEqual([selected]);
  });

  it("is Needs you for an admin with an approval waiting, though a request is in progress", async () => {
    const { approvals } = requestFixtures;
    await mount(
      fakeController(requestsSnapshot({ approvals, entries: only("generating", "live"), role: "admin" }))
        .controller,
    );
    expect(selectedSection()[0]).toMatch(/^Needs you/);
  });

  it("does not move the person when a refresh brings in something for an earlier section", async () => {
    const fake = fakeController(requestsSnapshot({ entries: only("generating") }));
    await mount(fake.controller);
    await act(async () => fake.set(requestsSnapshot({ entries: only("failed", "generating") })));
    expect(selectedSection()).toEqual(["In progress, 1"]);
  });

  it("is the one last picked when the screen opens again, also after the composer", async () => {
    const fake = fakeController(requestsSnapshot({ entries: only("generating", "live") }));
    const first = await mount(fake.controller);
    await click(byLabel("Done, 1"));
    expect(selectedSection()).toEqual(["Done, 1"]);

    // Out to the composer and back keeps it.
    await click(byLabel("Request a channel"));
    await click(byLabel("Back to Requests"));
    expect(selectedSection()).toEqual(["Done, 1"]);

    // Out to another tab (the host unmounts this screen) and back keeps it too.
    await first.unmount();
    await mount(fake.controller);
    expect(selectedSection()).toEqual(["Done, 1"]);
  });

  it("starts over with a new controller, as after pairing again", async () => {
    const entries = only("generating", "live");
    await mount(fakeController(requestsSnapshot({ entries })).controller);
    await click(byLabel("Done, 1"));
    await mounted[0]?.unmount();
    await mount(fakeController(requestsSnapshot({ entries })).controller);
    expect(selectedSection()).toEqual(["In progress, 1"]);
  });
});

describe("a Done request", () => {
  it("is one row that names its channel and opens it, with no separate Open channel button", async () => {
    await mount(fakeController(requestsSnapshot({ entries: only("live") })).controller);
    const row = byLabel("Channel 14, Rainy Day Detectives. Requested 5 days ago");
    expect(row.textContent).toContain("14");
    expect(row.textContent).toContain("Rainy Day Detectives");
    expect(row.textContent).toContain("Requested 5 days ago");
    expect(doc.body.textContent).not.toContain("Open channel");
    await click(row);
    expect(noop).toHaveBeenCalledWith("channel-rainy");
  });

  it("is read by VoiceOver once: the row is the only element with a label", async () => {
    await mount(fakeController(requestsSnapshot({ entries: only("live") })).controller);
    const row = byLabel("Channel 14, Rainy Day Detectives. Requested 5 days ago");
    expect(row.getAttribute("role")).toBe("button");
    expect(row.querySelectorAll("[aria-label]")).toHaveLength(0);
  });

  it("falls back to the request's own words when the guide has no such channel", async () => {
    await mount(fakeController(requestsSnapshot({ entries: only("live") })).controller, []);
    expect(
      byLabel("Gentle detective stories for rainy evenings. Requested 5 days ago").getAttribute("role"),
    ).toBe("button");
  });

  it("leaves a declined request as a card that opens its reason", async () => {
    await mount(fakeController(requestsSnapshot({ entries: only("denied") })).controller);
    expect(all('[aria-label$="Not approved"]')).toHaveLength(1);
  });
});

describe("pull to refresh", () => {
  const latest = () => {
    const control = refreshControls.at(-1);
    if (!control?.onRefresh) throw new Error("no refresh control on the list");
    return control as Required<Pick<typeof control, "onRefresh">> & typeof control;
  };

  it("refetches the list and spins until it has", async () => {
    const fake = fakeController(requestsSnapshot({ entries: only("generating") }));
    let finish: () => void = () => undefined;
    vi.mocked(fake.controller.refresh).mockImplementation(() => new Promise<void>((done) => (finish = done)));
    await mount(fake.controller);
    // Opening the screen reads the list once.
    vi.mocked(fake.controller.refresh).mockClear();
    expect(latest().refreshing).toBe(false);

    await act(async () => latest().onRefresh());
    expect(fake.controller.refresh).toHaveBeenCalledTimes(1);
    expect(latest().refreshing).toBe(true);

    await act(async () => finish());
    expect(latest().refreshing).toBe(false);
  });

  it("shows a request made after the screen opened", async () => {
    const fake = fakeController(requestsSnapshot({ entries: only("generating") }));
    await mount(fake.controller);
    expect(doc.body.textContent).not.toContain("Waiting for an admin");
    vi.mocked(fake.controller.refresh).mockImplementation(async () =>
      fake.set(requestsSnapshot({ entries: only("generating", "awaitingApproval") })),
    );
    await act(async () => latest().onRefresh());
    expect(selectedSection()).toEqual(["In progress, 2"]);
  });
});

describe("the composer's refusal banner", () => {
  it("is dismissed when the composer closes, so the list never shows it", async () => {
    const fake = fakeController(requestsSnapshot({ notice: "Admins only" }));
    await mount(fake.controller);
    await click(byLabel("Request a channel"));
    vi.mocked(fake.controller.dismissNotice).mockClear();
    await click(byLabel("Back to Requests"));
    expect(fake.controller.dismissNotice).toHaveBeenCalledTimes(1);
  });

  it("is not dismissed by leaving a request's detail, which never showed it", async () => {
    const fake = fakeController(requestsSnapshot({ entries: only("generating") }));
    await mount(fake.controller);
    await click(
      byLabel(`${journeys.generating.intent.description}, ${requestsSnapshot().entries[1]?.status.line}`),
    );
    await click(byLabel("Back to Requests"));
    expect(fake.controller.dismissNotice).not.toHaveBeenCalled();
  });
});
