import { ExcludedItemDTOReason } from "@loomarr/api";
import { ScheduleFactDTOReason } from "@loomarr/api/models/scheduleFactDTOReason";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ChannelCyclePreview } from "./channel-cycle-preview";

// ChannelCyclePreview drives channelsApi.usePreviewChannelCycle directly (no fetch layer
// worth stubbing for a pure read like RefinePanel's chat calls) — mock the hook itself,
// asserting on what params it was called with, the same way a component-level API mock
// is used elsewhere in this app for a hook whose shape (not its wire request) is what a
// unit test cares about.
const mockUsePreviewChannelCycle = vi.fn();
const mockUsePreviewChannelProgramming = vi.fn();
vi.mock("@loomarr/api/endpoints/channels", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@loomarr/api/endpoints/channels")>();
  return {
    ...actual,
    usePreviewChannelCycle: (...args: unknown[]) => mockUsePreviewChannelCycle(...args),
    usePreviewChannelProgramming: (...args: unknown[]) => mockUsePreviewChannelProgramming(...args),
  };
});

afterEach(() => {
  mockUsePreviewChannelCycle.mockReset();
  mockUsePreviewChannelProgramming.mockReset();
});

// The draft path is a MUTATION, so its idle shape differs from a query's: no `refetch`, and
// `isPending` is false before it has ever fired.
const draftIdle = () => ({ data: undefined, isPending: false, error: null, mutate: vi.fn() });

const loaded = (body: {
  at: string;
  activeRule: { id: string; label: string; priority: number; matched: boolean };
  windowMs: number;
  slots: { kind: "program" | "pending" | "break"; title?: string; key?: string; part?: number }[];
  excluded?: {
    overCeiling: number;
    unrated: number;
    items: { key: string; title: string; reason: ExcludedItemDTOReason }[];
  };
  trace?: {
    version: number;
    ordering: "sequential" | "shuffle" | "syndication";
    seed: string;
    windowMs: number;
    windowIndex: number;
    relaxations: { kind: string; from: string; to: string }[];
    factTotal: number;
    recordedTotal: number;
    truncated: boolean;
    facts: {
      stage: "hard_filter" | "availability" | "episode_selection" | "placement";
      outcome:
        | "kept"
        | "excluded"
        | "available"
        | "pending"
        | "selected"
        | "omitted"
        | "placed"
        | "windowed_out"
        | "inserted";
      reason: ScheduleFactDTOReason;
      key?: string;
      title?: string;
      deckPosition?: number;
      cyclePosition?: number;
    }[];
  };
}) => ({
  // ⚠ `excluded` is DEFAULTED, not optional on the wire — the BE marks it required, so a body
  // reaching the component without it is a contract break, not a state to render around. The
  // default exists so the tests that predate it (and care about rules/slots) stay unedited;
  // it must NOT become a reason for the component to guard against a missing field.
  data: {
    status: 200,
    data: {
      excluded: { overCeiling: 0, unrated: 0, items: [] },
      trace: {
        version: 1,
        ordering: "sequential",
        seed: "0",
        windowMs: 0,
        windowIndex: 0,
        relaxations: [],
        factTotal: 0,
        recordedTotal: 0,
        truncated: false,
        facts: [],
      },
      ...body,
    },
  },
  isLoading: false,
  error: null,
  refetch: vi.fn(),
});

describe("ChannelCyclePreview", () => {
  // Every test renders the component, which calls BOTH hooks regardless of which path is
  // active — so the mutation mock needs a shape even when the saved path is under test.
  beforeEach(() => {
    mockUsePreviewChannelProgramming.mockReturnValue(draftIdle());
  });

  it("shows the matched rule attribution with its label and priority", () => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-12-25T09:00:00Z",
        activeRule: { id: "r1", label: "Christmas · Marathon", priority: 60, matched: true },
        windowMs: 0,
        slots: [],
      }),
    );

    render(<ChannelCyclePreview channelId="ch-1" />);

    expect(screen.getByText("Active rule:")).toBeInTheDocument();
    expect(screen.getByText("Christmas · Marathon")).toBeInTheDocument();
    expect(screen.getByText("(priority 60)")).toBeInTheDocument();
  });

  it("shows 'base policy' when no rule matched", () => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-07-24T14:00:00Z",
        activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
        windowMs: 24 * 60 * 60 * 1000,
        slots: [],
      }),
    );

    render(<ChannelCyclePreview channelId="ch-1" />);

    expect(screen.getByText(/no rule active: base policy/i)).toBeInTheDocument();
  });

  it("renders program, pending, and break slots distinctly", () => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-07-24T14:00:00Z",
        activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
        windowMs: 0,
        slots: [
          { kind: "program", title: "Die Hard", key: "movie:tmdb:562" },
          { kind: "break" },
          { kind: "pending", title: "Elf", key: "movie:tmdb:10719" },
          { kind: "program", title: "Home Alone 2", key: "movie:tmdb:772", part: 2 },
        ],
      }),
    );

    render(<ChannelCyclePreview channelId="ch-1" />);

    expect(screen.getByText("Die Hard")).toBeInTheDocument();
    expect(screen.getByText(/elf/i)).toBeInTheDocument();
    expect(screen.getByText(/coming soon/i)).toBeInTheDocument();
    expect(screen.getByText(/commercial break/i)).toBeInTheDocument();
    expect(screen.getByText("Part 2")).toBeInTheDocument();
  });

  it("shows scheduler-owned why-this and why-not facts", async () => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-07-24T14:00:00Z",
        activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
        windowMs: 24 * 60 * 60 * 1000,
        slots: [{ kind: "program", title: "Die Hard", key: "movie:tmdb:562" }],
        trace: {
          version: 1,
          ordering: "shuffle",
          seed: "42",
          windowMs: 24 * 60 * 60 * 1000,
          windowIndex: 7,
          relaxations: [],
          factTotal: 3,
          recordedTotal: 2,
          truncated: true,
          facts: [
            {
              stage: "placement",
              outcome: "placed",
              reason: ScheduleFactDTOReason.shuffle,
              key: "movie:tmdb:562",
              title: "Die Hard",
              deckPosition: 4,
              cyclePosition: 0,
            },
            {
              stage: "availability",
              outcome: "pending",
              reason: ScheduleFactDTOReason.unavailable_pod_fill,
              key: "movie:tmdb:10719",
              title: "Elf",
            },
          ],
        },
      }),
    );

    render(<ChannelCyclePreview channelId="ch-1" />);
    expect(screen.getByText(/2 recorded decisions/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /show scheduler decisions/i }));
    expect(screen.getByText(/placed by the deterministic shuffle/i)).toBeInTheDocument();
    expect(screen.getByText(/waiting for acquisition; filler holds its place/i)).toBeInTheDocument();
    expect(screen.getByText(/1 additional decision omitted/i)).toBeInTheDocument();
  });

  it("shows the whole-run horizon when windowMs is 0", () => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-12-25T09:00:00Z",
        activeRule: { id: "r1", label: "Marathon", priority: 60, matched: true },
        windowMs: 0,
        slots: [],
      }),
    );

    render(<ChannelCyclePreview channelId="ch-1" />);
    expect(screen.getByText(/whole run/i)).toBeInTheDocument();
  });

  it("shows an empty state when no slots resolve", () => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-07-24T14:00:00Z",
        activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
        windowMs: 0,
        slots: [],
      }),
    );

    render(<ChannelCyclePreview channelId="ch-1" />);
    expect(screen.getByText(/nothing scheduled/i)).toBeInTheDocument();
  });

  it("clicking a time preset changes the `at` param passed to the hook", async () => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-07-24T14:00:00Z",
        activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
        windowMs: 0,
        slots: [],
      }),
    );

    render(<ChannelCyclePreview channelId="ch-1" />);

    // Initial render: no `at` yet ("Now"), so the params arg is undefined. The third arg is
    // the react-query options carrying `enabled` — the saved query is DISABLED whenever a
    // draft is being previewed, so it is asserted rather than ignored.
    expect(mockUsePreviewChannelCycle).toHaveBeenLastCalledWith("ch-1", undefined, {
      query: { enabled: true },
    });

    await userEvent.click(screen.getByRole("button", { name: "Christmas morning" }));

    const lastCall = mockUsePreviewChannelCycle.mock.calls.at(-1);
    expect(lastCall?.[0]).toBe("ch-1");
    expect(lastCall?.[1]).toEqual(expect.objectContaining({ at: expect.any(String) }));
    expect(String(lastCall?.[1]?.at)).toMatch(/^\d{4}-\d{2}-\d{2}T/);
  });

  it("shows a loading state", () => {
    mockUsePreviewChannelCycle.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    });

    render(<ChannelCyclePreview channelId="ch-1" />);
    expect(screen.getByText(/loading preview/i)).toBeInTheDocument();
  });

  it("shows an error state with retry", async () => {
    const refetch = vi.fn();
    mockUsePreviewChannelCycle.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("boom"),
      refetch,
    });

    render(<ChannelCyclePreview channelId="ch-1" />);
    expect(screen.getByRole("alert")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /try again/i }));
    expect(refetch).toHaveBeenCalledTimes(1);
  });
});

// ⚠ THE DRAFT PATH (programming-design §8.1). Editing a rule and watching the pane beneath it
// show the OLD schedule is worse than no preview at all — it reads as the edit having done
// nothing. These pin that a draft replaces the saved read rather than sitting beside it.
describe("ChannelCyclePreview — unsaved draft", () => {
  beforeEach(() => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-07-30T12:00:00Z",
        activeRule: { id: "saved", label: "Saved rule", priority: 1, matched: true },
        windowMs: 0,
        slots: [],
      }),
    );
  });

  it("previews the DRAFT instead of the saved channel, and says so", async () => {
    mockUsePreviewChannelProgramming.mockReturnValue({
      ...draftIdle(),
      data: {
        status: 200,
        data: {
          at: "2026-07-30T12:00:00Z",
          activeRule: { id: "draft", label: "Weekend marathon", priority: 5, matched: true },
          windowMs: 0,
          slots: [],
          // Hand-built rather than via `loaded()` (this is the MUTATION shape, not the query's),
          // so it needs the required field spelled out — the BE sends it on both endpoints.
          excluded: { overCeiling: 0, unrated: 0, items: [] },
          trace: {
            version: 1,
            ordering: "shuffle",
            seed: "0",
            windowMs: 0,
            windowIndex: 0,
            relaxations: [],
            factTotal: 0,
            recordedTotal: 0,
            truncated: false,
            facts: [],
          },
        },
      },
    });

    render(<ChannelCyclePreview channelId="ch-1" draftPolicy={{ ordering: "shuffle" }} />);

    // The DRAFT's rule is shown, not the saved one — the assertion that the pane actually
    // switched source rather than merely rendering.
    expect(screen.getByText("Weekend marathon")).toBeInTheDocument();
    expect(screen.queryByText("Saved rule")).not.toBeInTheDocument();
    // And the reader is told which schedule they are looking at.
    expect(screen.getByText(/unsaved changes/i)).toBeInTheDocument();
  });

  it("disables the saved query while a draft is being previewed", () => {
    mockUsePreviewChannelProgramming.mockReturnValue(draftIdle());

    render(<ChannelCyclePreview channelId="ch-1" draftPolicy={{ ordering: "shuffle" }} />);

    // ⚠ Not cosmetic: leaving it enabled would fire a second request per keystroke-settle
    // for a result nothing renders.
    expect(mockUsePreviewChannelCycle).toHaveBeenLastCalledWith("ch-1", undefined, {
      query: { enabled: false },
    });
  });

  // --- "why isn't X on my channel" (#263) ---------------------------------------------------
  //
  // The §4 exclusion report is computed on EVERY reconcile and was discarded by every caller
  // until this panel. These pin that it reaches the screen, because the failure mode it replaces
  // was silence — a refused title simply not appearing, with nothing anywhere saying why.

  it("names each refused title and why, once expanded", async () => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-07-24T14:00:00Z",
        activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
        windowMs: 0,
        slots: [{ kind: "program", title: "Rugrats", key: "series:tmdb:3022" }],
        excluded: {
          overCeiling: 2,
          unrated: 0,
          items: [
            {
              key: "series:tmdb:2190",
              title: "South Park",
              reason: ExcludedItemDTOReason.over_ceiling,
            },
            // Two rows sharing ONE key — a per-episode ceiling refusal carries its series key.
            // If the list keyed on `key` alone, React would drop one of these silently.
            {
              key: "series:tmdb:2122",
              title: "King of the Hill S04E06",
              reason: ExcludedItemDTOReason.over_ceiling,
            },
          ],
        },
      }),
    );

    render(<ChannelCyclePreview channelId="ch-1" />);

    // The count is visible WITHOUT expanding — folded, not hidden.
    expect(screen.getByText(/2 titles not airing/i)).toBeInTheDocument();
    expect(screen.getByText(/2 held back by the audience ceiling/i)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /show which titles are not airing/i }));

    expect(screen.getByText("South Park")).toBeInTheDocument();
    expect(screen.getByText("King of the Hill S04E06")).toBeInTheDocument();
    // The REASON is the whole point — a list of names alone answers "what", not "why".
    expect(screen.getAllByText(/rated above this channel's audience ceiling/i)).toHaveLength(2);
  });

  it("says nothing at all when the filters refused nothing", () => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-07-24T14:00:00Z",
        activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
        windowMs: 0,
        slots: [{ kind: "program", title: "Rugrats", key: "series:tmdb:3022" }],
        excluded: { overCeiling: 0, unrated: 0, items: [] },
      }),
    );

    render(<ChannelCyclePreview channelId="ch-1" />);

    // ⚠ Absent, not "0 titles not airing". A healthy channel is the overwhelming majority case,
    // and a permanent zero-row trains the eye to skip the panel exactly when it matters.
    expect(screen.queryByText(/not airing/i)).not.toBeInTheDocument();
  });

  it("distinguishes a taste filter from the safety gate in its summary", () => {
    mockUsePreviewChannelCycle.mockReturnValue(
      loaded({
        at: "2026-07-24T14:00:00Z",
        activeRule: { id: "", label: "Base policy", priority: 0, matched: false },
        windowMs: 0,
        slots: [],
        excluded: {
          overCeiling: 0,
          unrated: 0,
          items: [{ key: "movie:tmdb:1", title: "Too Old", reason: ExcludedItemDTOReason.out_of_scope }],
        },
      }),
    );

    render(<ChannelCyclePreview channelId="ch-1" />);

    // An era/genre drop is a curation choice the operator made, not a guardrail firing — so the
    // summary must NOT claim the ceiling held anything back.
    expect(screen.getByText(/filtered out by this channel's rules/i)).toBeInTheDocument();
    expect(screen.queryByText(/held back by the audience ceiling/i)).not.toBeInTheDocument();
  });

  it("without a draft, behaves exactly as before", () => {
    mockUsePreviewChannelProgramming.mockReturnValue(draftIdle());

    render(<ChannelCyclePreview channelId="ch-1" />);

    // The saved rule renders and the draft badge is absent — the additive guarantee: a caller
    // that passes no draft cannot tell the prop exists.
    expect(screen.getByText("Saved rule")).toBeInTheDocument();
    expect(screen.queryByText(/unsaved changes/i)).not.toBeInTheDocument();
  });
});
