import type { PreviewProgrammingChangesOutputBody } from "@loomarr/api/models/previewProgrammingChangesOutputBody";
import type { ProgrammingChangeDTO } from "@loomarr/api/models/programmingChangeDTO";
import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PREVIEW_DEBOUNCE_MS } from "@/channels/use-channel-rules-draft";
import { ChannelProgrammingChanges } from "./channel-programming-changes";

// The hook is a mutation; tests drive its returned shape, not the wire.
const mockUseChanges = vi.fn();
vi.mock("@loomarr/api/endpoints/channels", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@loomarr/api/endpoints/channels")>();
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
    render(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    expect(screen.getByText("Loading changes…")).toBeInTheDocument();
  });

  it("shows an error with a retry that re-posts the draft", () => {
    mockUseChanges.mockReturnValue(state({ error: new Error("boom") }));
    render(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    act(() => screen.getByRole("button", { name: /try again/i }).click());
    expect(mutate).toHaveBeenCalledWith({ id: "ch-1", data: { policy: draft } });
  });

  it("tells 'nothing changes' apart from 'nothing scheduled'", () => {
    mockUseChanges.mockReturnValue(state({ data: body({ compared: 30, count: 0 }) }));
    const { unmount } = render(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    expect(screen.getByText("No upcoming slots change")).toBeInTheDocument();
    unmount();

    mockUseChanges.mockReturnValue(state({ data: body({ compared: 0, count: 0 }) }));
    render(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    expect(screen.getByText("Nothing scheduled")).toBeInTheDocument();
  });

  it("lists each changed slot with its time, before → after, and the winning rule", () => {
    mockUseChanges.mockReturnValue(state({ data: body({ count: 2, changes: [change(0), change(1)] }) }));
    render(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} timeZone="UTC" />);
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
    render(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} timeZone="UTC" />);
    const rows = screen.getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("Nothing airs");
    expect(rows[0]).toHaveTextContent("Signal and Noise · S01E02 — Pilot");
    expect(rows[1]).toHaveTextContent("Commercial break");
  });

  it("says how many rows were cut when the list is truncated", () => {
    const shown = Array.from({ length: 100 }, (_, i) => change(i));
    mockUseChanges.mockReturnValue(state({ data: body({ count: 150, truncated: true, changes: shown }) }));
    render(<ChannelProgrammingChanges channelId="ch-1" draftPolicy={draft} />);
    expect(screen.getByText(/^Showing 100 of 150 changes until/)).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(100);
  });
});
