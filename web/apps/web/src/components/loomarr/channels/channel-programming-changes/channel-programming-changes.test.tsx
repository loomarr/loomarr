import type { PreviewProgrammingChangesOutputBody } from "@loomarr/api/models/previewProgrammingChangesOutputBody";
import type { ProgrammingChangeDTO } from "@loomarr/api/models/programmingChangeDTO";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PREVIEW_DEBOUNCE_MS } from "@/channels/use-channel-rules-draft";
import { ChannelProgrammingChanges } from "./channel-programming-changes";

// Most tests drive the mutation's returned shape, not the wire. The last block swaps the real
// hook back in to prove the request timing against a stubbed fetch.
const mockUseChanges = vi.fn();
const realHook = vi.hoisted(() => ({ use: undefined as undefined | ((...args: unknown[]) => unknown) }));
vi.mock("@loomarr/api/endpoints/channels", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@loomarr/api/endpoints/channels")>();
  realHook.use = actual.usePreviewChannelProgrammingChanges as (...args: unknown[]) => unknown;
  return {
    ...actual,
    usePreviewChannelProgrammingChanges: (...args: unknown[]) => mockUseChanges(...args),
  };
});

const draft = { rules: [{ id: "r1", label: "Sunday horror", priority: 10 }] };
const mutate = vi.fn();

const state = (over: {
  data?: PreviewProgrammingChangesOutputBody;
  isPending?: boolean;
  error?: unknown;
}) => ({
  data: over.data ? { status: 200, data: over.data } : undefined,
  isPending: over.isPending ?? false,
  error: over.error ?? null,
  mutate,
  reset: vi.fn(),
});

// Sunday 2026-10-04 18:00 UTC.
const SIX_PM = Date.UTC(2026, 9, 4, 18);
const HOUR = 60 * 60 * 1000;

const change = (i: number): ProgrammingChangeDTO => ({
  startMs: SIX_PM + i * HOUR,
  endMs: SIX_PM + (i + 1) * HOUR,
  before: {
    kind: "program",
    title: `Saved ${i}`,
    key: `movie:tmdb:${i}`,
    startMs: SIX_PM + i * HOUR,
    stopMs: SIX_PM + (i + 1) * HOUR,
    rule: { id: "", label: "Base policy", priority: 0, matched: false },
  },
  after: {
    kind: "program",
    title: `Draft ${i}`,
    key: `movie:tmdb:${100 + i}`,
    startMs: SIX_PM + i * HOUR,
    stopMs: SIX_PM + (i + 1) * HOUR,
    rule: { id: "r1", label: "Sunday horror", priority: 10, matched: true },
  },
});

const body = (over: Partial<PreviewProgrammingChangesOutputBody>): PreviewProgrammingChangesOutputBody => ({
  fromMs: SIX_PM,
  toMs: SIX_PM + 24 * HOUR,
  compared: 30,
  count: 0,
  truncated: false,
  changes: [],
  ...over,
});

beforeEach(() => mutate.mockReset());

// A result only counts once its draft's request has been sent, so render past the debounce.
const settled = (ui: React.ReactElement) => {
  vi.useFakeTimers();
  const view = render(ui);
  act(() => vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS));
  return view;
};
afterEach(() => {
  mockUseChanges.mockReset();
  vi.useRealTimers();
});

describe("ChannelProgrammingChanges", () => {
  it("sends nothing and says why when nothing is unsaved", () => {
    vi.useFakeTimers();
    mockUseChanges.mockReturnValue(state({}));
    render(<ChannelProgrammingChanges channelId="ch-1" />);
    act(() => vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS * 2));
    expect(mutate).not.toHaveBeenCalled();
    expect(screen.getByText(/edit the rules above/i)).toBeInTheDocument();
  });

  it("posts the draft after the debounce, leaving `from` to the server", () => {
    vi.useFakeTimers();
    mockUseChanges.mockReturnValue(state({}));
    render(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    expect(mutate).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS));
    expect(mutate).toHaveBeenCalledWith({ id: "ch-1", data: { policy: draft } });
  });

  it("shows loading while the comparison runs", () => {
    mockUseChanges.mockReturnValue(state({ isPending: true }));
    settled(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    expect(screen.getByText("Loading changes…")).toBeInTheDocument();
  });

  it("shows an error with a retry that re-posts the draft", () => {
    mockUseChanges.mockReturnValue(state({ error: new Error("boom") }));
    settled(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    act(() => screen.getByRole("button", { name: /try again/i }).click());
    expect(mutate).toHaveBeenCalledWith({ id: "ch-1", data: { policy: draft } });
  });

  it("tells 'nothing changes' apart from 'nothing scheduled'", () => {
    mockUseChanges.mockReturnValue(state({ data: body({ compared: 30, count: 0 }) }));
    const { unmount } = settled(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    expect(screen.getByText("No upcoming slots change")).toBeInTheDocument();
    unmount();

    mockUseChanges.mockReturnValue(state({ data: body({ compared: 0, count: 0 }) }));
    settled(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    expect(screen.getByText("Nothing scheduled")).toBeInTheDocument();
  });

  it("lists each changed slot with its time, before → after, and the winning rule", () => {
    mockUseChanges.mockReturnValue(state({ data: body({ count: 2, changes: [change(0), change(1)] }) }));
    settled(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} timeZone="UTC" />);
    const rows = screen.getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Sun 6:00 PM");
    expect(rows[0]).toHaveTextContent("Saved 0");
    expect(rows[0]).toHaveTextContent("Draft 0");
    expect(rows[0]).toHaveTextContent("Sunday horror");
    expect(screen.getByText(/^2 changes until/)).toBeInTheDocument();
  });

  it("names episodes by series, breaks, and sides where nothing airs", () => {
    const episode: ProgrammingChangeDTO = {
      ...change(0),
      before: undefined,
      after: { ...change(0).after!, series: "Signal and Noise", title: "Pilot", season: 1, episode: 2 },
    };
    const toBreak: ProgrammingChangeDTO = {
      ...change(1),
      after: { ...change(1).after!, kind: "break", title: undefined, key: undefined },
    };
    mockUseChanges.mockReturnValue(state({ data: body({ count: 2, changes: [episode, toBreak] }) }));
    settled(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} timeZone="UTC" />);
    const rows = screen.getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("Nothing airs");
    expect(rows[0]).toHaveTextContent("Signal and Noise · S01E02 — Pilot");
    expect(rows[1]).toHaveTextContent("Commercial break");
  });

  it("says how many rows were cut when the list is truncated", () => {
    const shown = Array.from({ length: 100 }, (_, i) => change(i));
    mockUseChanges.mockReturnValue(state({ data: body({ count: 150, truncated: true, changes: shown }) }));
    settled(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    expect(screen.getByText(/^Showing 100 of 150 changes until/)).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(100);
  });
});

// The real mutation against a stubbed fetch: what reaches the wire, and what a new draft shows
// while its request is still pending.
describe("ChannelProgrammingChanges request timing", () => {
  const draftB = { rules: [{ id: "r2", label: "Weekday kids", priority: 20 }] };
  const respond = (payload: PreviewProgrammingChangesOutputBody) =>
    Promise.resolve(
      new Response(JSON.stringify(payload), { status: 200, headers: { "content-type": "application/json" } }),
    );
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    mockUseChanges.mockImplementation((...args: unknown[]) => realHook.use?.(...args));
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });
  afterEach(() => fetchSpy.mockRestore());

  const renderWithClient = (draftPolicy?: typeof draft) => {
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
    const ui = (policy?: typeof draft) => (
      <QueryClientProvider client={client}>
        <ChannelProgrammingChanges channelId="ch-1" draftPolicy={policy} timeZone="UTC" />
      </QueryClientProvider>
    );
    const view = render(ui(draftPolicy));
    return { rerender: (policy?: typeof draft) => view.rerender(ui(policy)) };
  };

  it("coalesces rapid edits into one request for the last draft", async () => {
    fetchSpy.mockImplementation(() => respond(body({ count: 0 })));
    const { rerender } = renderWithClient(draft);
    act(() => vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS / 2));
    rerender(draftB);
    await act(async () => vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS));
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const init = fetchSpy.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(init.body))).toEqual({ policy: draftB });
  });

  it("hides the previous draft's rows while the next draft's request is pending", async () => {
    fetchSpy.mockImplementationOnce(() => respond(body({ count: 1, changes: [change(0)] })));
    const { rerender } = renderWithClient(draft);
    await act(async () => vi.advanceTimersByTime(PREVIEW_DEBOUNCE_MS));
    expect(await screen.findByText("Draft 0")).toBeInTheDocument();

    // The next draft never answers: its stale predecessor must not stand in for it.
    fetchSpy.mockImplementation(() => new Promise<Response>(() => {}));
    rerender(draftB);
    expect(screen.queryByText("Draft 0")).not.toBeInTheDocument();
    expect(screen.getByText("Loading changes…")).toBeInTheDocument();

    // Going clean drops it too, rather than keeping it for the next edit.
    rerender(undefined);
    expect(screen.getByText(/edit the rules above/i)).toBeInTheDocument();
  });
});
