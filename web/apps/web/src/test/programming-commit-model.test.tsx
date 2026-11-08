import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { UpdateChannelInputBody } from "@loomarr/api/models/updateChannelInputBody";
import {
  getChannelsNowNextMockHandler,
  getGetProgrammingVocabularyMockHandler,
  getListDiscoveryFeedbackMockHandler,
  getListLibraryCollectionsMockHandler,
  getMeMockHandler,
  getPreviewChannelCycleMockHandler,
  getPreviewChannelProgrammingMockHandler,
} from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import { ruleVocabularyFixture } from "@loomarr/fixtures";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { channel } from "@/test/fixtures/channels";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// design.md §12 names the review-before-apply surfaces, and scheduling RULES are the third —
// the only block of the Programming page that drafts. Rules resolve first-match-by-priority,
// so a rule's effect depends on every rule above it and each intermediate state is a different
// schedule; inline-saving each step would reconcile half-finished rule sets to Tunarr.
//
// The suite drives the real Programming route and watches the wire: a rules edit sends no
// PATCH until Apply, and every other block's edit PATCHes at once. The control half matters as
// much as the draft half: §12 scopes the exception to the interdependent surface, and a change
// that swept the self-contained blocks into the draft "for consistency" would restore the
// ceremony §12's first version removed.
const renderProgramming = (policy: ChannelPolicy = {}) => {
  // intent-backed, so auto-curate has an intent to re-run and offers its opt-in.
  const stored = { revision: 1, policy, intentRef: "intent-1" };
  const patches: UpdateChannelInputBody[] = [];
  const changePreviews: ChannelPolicy[] = [];
  server.use(
    getMeMockHandler(me()),
    // ⚠ Ahead of the by-id read, which would also answer `/v1/channels/now-next`.
    getChannelsNowNextMockHandler({ channels: [] }),
    getListDiscoveryFeedbackMockHandler([]),
    getListLibraryCollectionsMockHandler({ collections: [] }),
    getGetProgrammingVocabularyMockHandler(ruleVocabularyFixture),
    getPreviewChannelCycleMockHandler(),
    getPreviewChannelProgrammingMockHandler(),
    http.post("*/v1/channels/:id/programming/changes", async ({ request }) => {
      changePreviews.push(((await request.json()) as { policy: ChannelPolicy }).policy);
      return HttpResponse.json({ changes: [] } as never);
    }),
    http.get("*/v1/channels/:id", ({ params }) =>
      params.id === "ch-1" ? HttpResponse.json(channel({ id: "ch-1", ...stored })) : undefined,
    ),
    http.patch("*/v1/channels/:id", async ({ request }) => {
      const body = (await request.json()) as UpdateChannelInputBody;
      patches.push(body);
      if (body.policy) stored.policy = body.policy;
      stored.revision += 1;
      return HttpResponse.json(channel({ id: "ch-1", ...stored }));
    }),
    ...appHandlers(),
  );
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: ["/channels/ch-1/programming"] }),
  });
  render(
    <LoomarrProvider theme="dark">
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </LoomarrProvider>,
  );
  return { patches, changePreviews };
};

const open = async (user: ReturnType<typeof userEvent.setup>, section: string) => {
  await user.click(await screen.findByRole("button", { name: new RegExp(section, "i") }));
};

// Each test renders the whole route tree; the first pays the cold load of the Programming route,
// so the suite carries its own bound instead of the 5 s unit default.
describe("the Programming surface's commit model", () => {
  it("keeps a rules edit as a draft until Apply, and previews the draft meanwhile", async () => {
    const { patches, changePreviews } = renderProgramming();
    const user = userEvent.setup();
    await open(user, "When it changes");

    await user.click(await screen.findByRole("button", { name: /add rule/i }));

    const apply = await screen.findByRole("button", { name: "Apply rules" });
    expect(screen.getByRole("button", { name: "Discard" })).toBeInTheDocument();
    expect(screen.getByText(/Previewing the unsaved rules draft/)).toBeInTheDocument();
    // The preview follows the draft: what applying would change is asked of the server with the
    // drafted rule in it.
    await waitFor(() => expect(changePreviews.at(-1)?.rules).toHaveLength(1));
    expect(patches).toEqual([]);

    await user.click(apply);

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]?.policy?.rules).toHaveLength(1);
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Apply rules" })).not.toBeInTheDocument(),
    );
  });

  it("discards a rules draft without saving it", async () => {
    const { patches } = renderProgramming();
    const user = userEvent.setup();
    await open(user, "When it changes");

    await user.click(await screen.findByRole("button", { name: /add rule/i }));
    await user.click(await screen.findByRole("button", { name: "Discard" }));

    expect(screen.queryByRole("button", { name: "Apply rules" })).not.toBeInTheDocument();
    expect(screen.getByText(/no rules yet/i)).toBeInTheDocument();
    expect(patches).toEqual([]);
  });

  it("saves the ordering inline, not through the draft", async () => {
    const { patches } = renderProgramming();
    const user = userEvent.setup();
    await open(user, "How it's ordered");

    await user.selectOptions(await screen.findByRole("combobox", { name: "Play order" }), "In order");

    await waitFor(() => expect(patches.at(-1)?.policy?.ordering).toBe("sequential"));
    expect(screen.queryByRole("button", { name: "Apply rules" })).not.toBeInTheDocument();
  });

  it("saves the era scope inline, not through the draft", async () => {
    const { patches } = renderProgramming();
    const user = userEvent.setup();

    await user.type(await screen.findByLabelText("From year"), "1990");
    await user.tab();

    await waitFor(() => expect(patches.at(-1)?.policy?.scope?.dates).toBeDefined());
    expect(screen.queryByRole("button", { name: "Apply rules" })).not.toBeInTheDocument();
  });

  it("saves seasonal and auto-curate inline, not through the draft", async () => {
    const { patches } = renderProgramming();
    const user = userEvent.setup();
    await open(user, "When it changes");

    await user.click(await screen.findByRole("radio", { name: "Holiday-only channel" }));
    await waitFor(() => expect(patches.at(-1)?.policy?.seasonal).toBeDefined());

    await user.click(screen.getByRole("checkbox", { name: "Add new titles without asking" }));
    await waitFor(() => expect(patches.at(-1)?.policy?.autoCurate).toBeDefined());

    expect(screen.queryByRole("button", { name: "Apply rules" })).not.toBeInTheDocument();
  });
}, 20_000);
