import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";
import {
  getListChannelIdeasMockHandler,
  getSearchMockHandler,
  getSettingsListMockHandler,
  getSubmitManualChannelMockHandler,
} from "@loomarr/api/msw";
import { CHANNEL_TEMPLATES } from "@loomarr/core/templates";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render as rtlRender, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { POLL_MS } from "../use-ai-reconnect";
import { ManualChannelBuilder } from "./manual-channel-builder";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const render = (ui: ReactElement) => rtlRender(ui, { wrapper: makeWrapper() });

const comedyIdea: ChannelIdeaDTO = {
  id: "genre:comedy",
  name: "Comedy Movies",
  pitch: "Every comedy title in your library that no channel plays yet, on one channel.",
  reason: { kind: "unaired", count: 2 },
  facet: "genre",
  value: "Comedy",
  keys: ["movie:tmdb:1", "movie:tmdb:2"],
  titles: [
    {
      name: "Autumn Market",
      year: 2003,
      mediaType: "movie",
      tmdbId: 1,
      inLibrary: true,
      libraryItemId: "lib-1",
    },
    {
      name: "The Understudy",
      year: 1999,
      mediaType: "movie",
      tmdbId: 2,
      inLibrary: true,
      libraryItemId: "lib-2",
    },
  ],
  movies: 2,
  series: 0,
  inLibrary: 2,
  toDownload: 0,
  requested: false,
};

const stubIdeas = (ideas: ChannelIdeaDTO[]) => {
  server.use(getListChannelIdeasMockHandler({ ideas }));
};

// The manual/AI-off builder (#1817, G2): starter templates fill name/description only, library
// facets (the existing channel-ideas data) and search feed one lineup, and "Continue to review"
// is the one write — a new proposal, re-grounded server-side, never a second creation route.
describe("ManualChannelBuilder", () => {
  it("fills the name and description from a starter template, without touching the lineup", async () => {
    stubIdeas([]);
    const user = userEvent.setup();
    render(<ManualChannelBuilder isAdmin onCancel={vi.fn()} onSubmitted={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: /cozy mystery nights/i }));

    expect(screen.getByLabelText("Channel name")).toHaveValue("Cozy Mystery Nights");
    expect(screen.getByText(/no titles yet/i)).toBeInTheDocument();
  });

  it("adds a facet's titles one at a time and keeps already-added titles out of the list", async () => {
    stubIdeas([comedyIdea]);
    const user = userEvent.setup();
    render(<ManualChannelBuilder isAdmin onCancel={vi.fn()} onSubmitted={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: /comedy movies/i }));
    const addButtons = await screen.findAllByRole("button", { name: "Add" });
    expect(addButtons).toHaveLength(2);
    await user.click(addButtons[0]!);

    expect(screen.getByText("Autumn Market")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Add" })).toHaveLength(1);
  });

  it("adds a searched title to the lineup", async () => {
    stubIdeas([]);
    server.use(
      getSearchMockHandler({
        candidates: [{ name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true }],
      }),
    );
    const user = userEvent.setup();
    render(<ManualChannelBuilder isAdmin onCancel={vi.fn()} onSubmitted={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Search titles" }));
    await user.type(screen.getByPlaceholderText(/search for a movie or show/i), "heat");
    await user.click(await screen.findByText("Heat"));

    expect(screen.getByText("Heat")).toBeInTheDocument();
  });

  it("disables Continue to review until a name and at least one title are chosen", async () => {
    stubIdeas([]);
    render(<ManualChannelBuilder isAdmin onCancel={vi.fn()} onSubmitted={vi.fn()} />);
    expect(await screen.findByRole("button", { name: "Continue to review" })).toBeDisabled();
  });

  it("submits the hand-picked lineup and hands the new job id to the caller", async () => {
    stubIdeas([comedyIdea]);
    server.use(getSubmitManualChannelMockHandler({ jobId: "job-manual-1" }));
    const onSubmitted = vi.fn();
    const user = userEvent.setup();
    render(<ManualChannelBuilder isAdmin onCancel={vi.fn()} onSubmitted={onSubmitted} />);

    await user.type(screen.getByLabelText("Channel name"), "My Hand-Built Channel");
    await user.click(await screen.findByRole("button", { name: /comedy movies/i }));
    await user.click((await screen.findAllByRole("button", { name: "Add" }))[0]!);
    await user.click(screen.getByRole("button", { name: "Continue to review" }));

    await waitFor(() => expect(onSubmitted).toHaveBeenCalledWith("job-manual-1"));
  });

  it("starting from an idea ('Edit first') hides templates and facets and preloads the lineup", async () => {
    render(
      <ManualChannelBuilder
        isAdmin={false}
        initialIdea={comedyIdea}
        onCancel={vi.fn()}
        onSubmitted={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("Channel name")).toHaveValue("Comedy Movies");
    expect(screen.getByText("Autumn Market")).toBeInTheDocument();
    expect(screen.getByText("The Understudy")).toBeInTheDocument();
    expect(screen.queryByText("Starter templates")).not.toBeInTheDocument();
    expect(screen.queryByText("Build from your library")).not.toBeInTheDocument();
  });

  it("Cancel hands control back without submitting", async () => {
    stubIdeas([]);
    const onCancel = vi.fn();
    const user = userEvent.setup();
    render(<ManualChannelBuilder isAdmin onCancel={onCancel} onSubmitted={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalled();
  });
});

// The resume banner (#1817, mock #1872 Alt A): AI comes back while an admin is building by hand.
describe("ManualChannelBuilder resume banner", () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
  afterEach(() => vi.useRealTimers());

  const renderWithTheme = (ui: ReactElement) => render(<LoomarrProvider theme="dark">{ui}</LoomarrProvider>);

  // GET /v1/settings' computed `features.suggestions`, flipped by the test; reads are counted.
  const stubFeatures = (initial: boolean) => {
    const state = { suggestions: initial, reads: 0 };
    server.use(
      getSettingsListMockHandler(() => {
        state.reads += 1;
        return { settings: [], features: { suggestions: state.suggestions } };
      }),
    );
    return state;
  };

  const reconnect = async (features: { suggestions: boolean; reads: number }) => {
    await waitFor(() => expect(features.reads).toBe(1));
    features.suggestions = true;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_MS);
    });
    return screen.findByText("AI is connected now");
  };

  const setupUser = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

  it("appears when AI comes back, and Try AI suggestions records the picks before handing over", async () => {
    const features = stubFeatures(false);
    let submitted: unknown;
    server.use(
      getSubmitManualChannelMockHandler(async ({ request }) => {
        submitted = await request.json();
        return { jobId: "job-manual-2" };
      }),
    );
    const onAiSubmitted = vi.fn();
    const user = setupUser();
    renderWithTheme(
      <ManualChannelBuilder
        isAdmin
        initialIdea={comedyIdea}
        onCancel={vi.fn()}
        onSubmitted={vi.fn()}
        onAiSubmitted={onAiSubmitted}
        onDescribeWithAi={vi.fn()}
      />,
    );
    expect(screen.queryByText("AI is connected now")).not.toBeInTheDocument();

    await reconnect(features);
    expect(
      screen.getByText(
        "Your manual lineup (2 titles) is still here. Keep building it by hand, or let Loomarr suggest titles for this description — your picks stay in the lineup either way.",
      ),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Try AI suggestions" }));

    await waitFor(() => expect(onAiSubmitted).toHaveBeenCalledWith("job-manual-2", comedyIdea.pitch));
    expect(submitted).toEqual({
      name: "Comedy Movies",
      description: comedyIdea.pitch,
      lineup: comedyIdea.titles,
    });
  });

  it("with nothing picked yet, sends the description back to the describe form instead", async () => {
    stubIdeas([]);
    const features = stubFeatures(false);
    const onDescribeWithAi = vi.fn();
    const onAiSubmitted = vi.fn();
    const user = setupUser();
    renderWithTheme(
      <ManualChannelBuilder
        isAdmin
        onCancel={vi.fn()}
        onSubmitted={vi.fn()}
        onAiSubmitted={onAiSubmitted}
        onDescribeWithAi={onDescribeWithAi}
      />,
    );
    await user.click(await screen.findByRole("button", { name: /cozy mystery nights/i }));

    await reconnect(features);
    expect(screen.getByText(/Your manual lineup \(0 titles\) is still here/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Try AI suggestions" }));

    expect(onDescribeWithAi).toHaveBeenCalledWith(
      CHANNEL_TEMPLATES.find((t) => t.label === "Cozy Mystery Nights")?.description,
    );
    expect(onAiSubmitted).not.toHaveBeenCalled();
  });

  it("holds Try AI suggestions with a reason until the picked lineup has a name", async () => {
    const features = stubFeatures(false);
    const user = setupUser();
    renderWithTheme(
      <ManualChannelBuilder
        isAdmin
        initialIdea={comedyIdea}
        onCancel={vi.fn()}
        onSubmitted={vi.fn()}
        onAiSubmitted={vi.fn()}
        onDescribeWithAi={vi.fn()}
      />,
    );
    await reconnect(features);

    await user.clear(screen.getByLabelText("Channel name"));

    expect(screen.getByRole("button", { name: "Try AI suggestions" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByText("Name this channel to try AI suggestions.")).toBeInTheDocument();
  });

  it("Keep building manually hides the banner for the session and stops polling", async () => {
    const features = stubFeatures(false);
    const user = setupUser();
    renderWithTheme(
      <ManualChannelBuilder
        isAdmin
        initialIdea={comedyIdea}
        onCancel={vi.fn()}
        onSubmitted={vi.fn()}
        onAiSubmitted={vi.fn()}
        onDescribeWithAi={vi.fn()}
      />,
    );
    await reconnect(features);

    await user.click(screen.getByRole("button", { name: "Keep building manually" }));
    expect(screen.queryByText("AI is connected now")).not.toBeInTheDocument();

    const reads = features.reads;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_MS * 4);
    });
    expect(features.reads).toBe(reads);
    expect(screen.getByText("Autumn Market")).toBeInTheDocument();
  });

  it("a member never polls the admin-only signal", async () => {
    const features = stubFeatures(false);
    renderWithTheme(
      <ManualChannelBuilder
        isAdmin={false}
        initialIdea={comedyIdea}
        onCancel={vi.fn()}
        onSubmitted={vi.fn()}
        onAiSubmitted={vi.fn()}
        onDescribeWithAi={vi.fn()}
      />,
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_MS * 4);
    });
    expect(features.reads).toBe(0);
  });
});
