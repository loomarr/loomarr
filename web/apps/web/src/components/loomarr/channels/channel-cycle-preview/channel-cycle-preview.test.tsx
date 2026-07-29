import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChannelCyclePreview } from "./channel-cycle-preview";

// ChannelCyclePreview drives channelsApi.usePreviewChannelCycle directly (no fetch layer
// worth stubbing for a pure read like RefinePanel's chat calls) — mock the hook itself,
// asserting on what params it was called with, the same way a component-level API mock
// is used elsewhere in this app for a hook whose shape (not its wire request) is what a
// unit test cares about.
const mockUsePreviewChannelCycle = vi.fn();
vi.mock("@loomarr/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@loomarr/api")>();
  return {
    ...actual,
    channelsApi: {
      ...actual.channelsApi,
      usePreviewChannelCycle: (...args: unknown[]) => mockUsePreviewChannelCycle(...args),
    },
  };
});

afterEach(() => {
  mockUsePreviewChannelCycle.mockReset();
});

const loaded = (body: {
  at: string;
  activeRule: { id: string; label: string; priority: number; matched: boolean };
  windowMs: number;
  slots: { kind: "program" | "pending" | "break"; title?: string; key?: string; part?: number }[];
}) => ({
  data: { status: 200, data: body },
  isLoading: false,
  error: null,
  refetch: vi.fn(),
});

describe("ChannelCyclePreview", () => {
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

    // Initial render: no `at` yet ("Now"), so the params arg is undefined.
    expect(mockUsePreviewChannelCycle).toHaveBeenLastCalledWith("ch-1", undefined);

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
