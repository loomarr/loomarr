import type { RequestsController, RequestsSnapshot } from "@loomarr/core/requests";
import { LoomarrProvider } from "@loomarr/design-system";
import { requestFixtures, requestsSnapshot } from "@loomarr/fixtures";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  ApprovalGroup,
  initialReview,
  RequestChannel,
  type RequestChannelProps,
  RequestDetail,
  RequestsJourney,
  RequestsList,
  type RequestsListProps,
  ReviewSheet,
  type ReviewSheetProps,
  type ReviewState,
  reviewReducer,
} from "../index";

const render = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

const { approvals, fillerPulls, ideas, journeys } = requestFixtures;
const admin = (over: Partial<RequestsSnapshot> = {}) =>
  requestsSnapshot({ approvals, fillerPulls, role: "admin", ...over });

const noop = vi.fn();
const list = (over: Partial<RequestsListProps>) =>
  render(
    <RequestsList
      onApprove={noop}
      onApproveFiller={noop}
      onFix={noop}
      onOpen={noop}
      onOpenChannel={noop}
      onRequestChannel={noop}
      onReview={noop}
      onRetry={noop}
      onTabChange={noop}
      review={initialReview}
      snapshot={requestsSnapshot()}
      tab="in-progress"
      {...over}
    />,
  );

describe("requests list", () => {
  it("says it is loading", () => {
    const markup = list({ snapshot: requestsSnapshot({ entries: [], status: "loading" }) });
    expect(markup).toContain("Loading your requests");
    expect(markup).toContain('aria-busy="true"');
  });

  it("explains a failed load in the server's words, and offers one retry", () => {
    const markup = list({
      snapshot: requestsSnapshot({ entries: [], errorMessage: "The server didn't answer.", status: "error" }),
    });
    expect(markup).toContain("Couldn&#x27;t load your requests.");
    expect(markup).toContain("Check your connection and try again.");
    expect(markup).toContain(">Try again<");
    expect(markup).toContain('role="alert"');
  });

  it("invites the first request when there is nothing at all", () => {
    const markup = list({ snapshot: requestsSnapshot({ entries: [] }) });
    expect(markup).toContain("You haven&#x27;t requested a channel yet");
    expect(markup).toContain(">Request a channel<");
    expect(markup).not.toContain('role="tablist"');
  });

  it("shows the three counted tabs with the active one selected", () => {
    const markup = list({});
    expect(markup).toContain('aria-label="Requests sections"');
    expect(markup.match(/role="tab"/g)).toHaveLength(3);
    expect(markup).toContain("Needs you (1)");
    expect(markup).toContain("In progress (3)");
    expect(markup).toContain("Done (2)");
    expect(markup.match(/aria-selected="true"/g)).toHaveLength(1);
  });

  it("lists in-progress requests with Web's status lines", () => {
    const markup = list({});
    expect(markup).toContain("Slow-burn space operas with big ships");
    expect(markup).toContain(">Generating<");
    expect(markup).toContain(">Waiting for approval<");
    expect(markup).toContain("Getting 2 titles (1 downloading, 1 waiting)");
    expect(markup).toContain("Requested ");
    // The mock's row order: the badge first, then the title.
    expect(markup.indexOf(">Generating<")).toBeLessThan(
      markup.indexOf(">Slow-burn space operas with big ships<"),
    );
    // Every card is a whole-row target at least 48 pt tall.
    expect(markup).toContain("min-height:48px");
  });

  it("lists done requests with Open channel, and a declined one", () => {
    const markup = list({ tab: "done" });
    expect(markup).toContain(">On your channel<");
    expect(markup).toContain(">Open channel<");
    expect(markup).toContain(">Not approved<");
  });

  // Each empty tab is one tab having nothing: the other tabs still hold a request.
  it.each([
    ["in-progress", "Nothing in progress", "live"],
    ["done", "Nothing finished yet", "generating"],
    ["needs-you", "Nothing needs you", "live"],
  ] as const)("ends an empty %s tab in the one next action", (tab, title, held) => {
    const entries = requestsSnapshot().entries.filter(
      (entry) => entry.journey.jobId === journeys[held].jobId,
    );
    const markup = list({ snapshot: requestsSnapshot({ entries }), tab });
    expect(markup).toContain(title);
    expect(markup).toContain(">Request a channel<");
  });

  describe("Needs you", () => {
    it("gives a member only their failed requests, with the reason and one way to fix it", () => {
      const markup = list({ snapshot: requestsSnapshot({ approvals }), tab: "needs-you" });
      expect(markup).toContain("Couldn&#x27;t be built");
      expect(markup).toContain("Only 2 titles matched that description. A channel needs at least 5 titles.");
      expect(markup).toContain(">Edit and try again<");
      expect(markup).not.toContain("Channel requests");
      expect(markup).not.toContain("Filler downloads");
      expect(markup).not.toContain(">Approve<");
      expect(markup).toContain("Needs you (1)");
    });

    it("keeps a member out of both groups even when the snapshot carries them", () => {
      const markup = list({ snapshot: requestsSnapshot({ approvals, fillerPulls }), tab: "needs-you" });
      expect(markup).not.toContain("Retro game-show bumpers");
      expect(markup).not.toContain("Grace Hopper");
    });

    it("gives an admin two groups first, then their own failures, counted together", () => {
      const markup = list({ snapshot: admin(), tab: "needs-you" });
      const at = (text: string) => markup.indexOf(text);
      expect(at("Channel requests")).toBeLessThan(at("Filler downloads"));
      expect(at("Filler downloads")).toBeLessThan(at("Couldn&#x27;t be built"));
      expect(markup).toContain("Needs you (6)");
      expect(markup).toContain("Requested by Grace Hopper");
      expect(markup.match(/>Approve</g)).toHaveLength(5);
    });

    it("words a channel request and a filler download each in its own terms", () => {
      const markup = list({ snapshot: admin(), tab: "needs-you" });
      expect(markup).toContain("1 title · 2 to download");
      expect(markup).toContain("1 title · all in your library");
      expect(markup).toContain("40 clips · 1 source");
      expect(markup).toContain("18 clips · 2 sources");
      expect(markup).toContain("Requested by Loomarr");
      expect(markup.match(/>Deny</g)).toHaveLength(3);
      expect(markup.match(/>Dismiss</g)).toHaveLength(2);
    });

    it("offers Edit on channel requests only", () => {
      expect(list({ snapshot: admin(), tab: "needs-you" }).match(/>Edit</g)).toHaveLength(3);
      expect(list({ snapshot: admin({ approvals: [] }), tab: "needs-you" })).not.toContain(">Edit<");
    });

    it("offers Select to a group from its second row, never one that spans both", () => {
      const markup = list({ snapshot: admin(), tab: "needs-you" });
      expect(markup.match(/>Select</g)).toHaveLength(2);
      expect(markup).not.toContain("Select all");
      expect(
        list({ snapshot: admin({ approvals: approvals.slice(0, 1), fillerPulls: [] }), tab: "needs-you" }),
      ).not.toContain(">Select<");
    });

    it("shows a group in Select mode as checkboxes, and only that group", () => {
      const review: ReviewState = {
        ...initialReview,
        selected: { filler: [], requests: ["approval-grace"] },
        selecting: { filler: false, requests: true },
      };
      const markup = list({ review, snapshot: admin(), tab: "needs-you" });
      expect(markup.match(/role="checkbox"/g)).toHaveLength(3);
      expect(markup).toContain(">Cancel<");
      // Filler downloads keep their own actions.
      expect(markup.match(/>Dismiss</g)).toHaveLength(2);
      expect(markup.match(/>Edit</g)).toBeNull();
    });

    it("lists a group on its own when the other is empty", () => {
      const markup = list({ snapshot: admin({ fillerPulls: [] }), tab: "needs-you" });
      expect(markup).toContain("Channel requests");
      expect(markup).not.toContain("Filler downloads");
      const fillerOnly = list({ snapshot: admin({ approvals: [] }), tab: "needs-you" });
      expect(fillerOnly).toContain("Filler downloads");
      expect(fillerOnly).not.toContain("Channel requests");
    });

    it("disables the decision being made", () => {
      const markup = list({ snapshot: admin({ deciding: ["approval-grace"] }), tab: "needs-you" });
      expect(markup).toContain(">Approving…<");
      expect(markup).toContain('aria-disabled="true"');
    });
  });
});

describe("approval group", () => {
  const rows = [
    {
      id: "a",
      meta: "14 titles · 3 to download",
      note: "For the film club.",
      requestedBy: "Jordan",
      title: "Sunday mornings",
    },
    { id: "b", meta: "6 titles · 1 to download", title: "Jazz and rain" },
  ];
  const group = (over = {}) =>
    render(
      <ApprovalGroup
        deciding={[]}
        onApprove={noop}
        onDeny={noop}
        onToggleSelected={noop}
        onToggleSelecting={noop}
        rows={rows}
        selected={[]}
        selecting={false}
        title="Channel requests"
        {...over}
      />,
    );

  it("shows what each row would do, who asked and their note", () => {
    const markup = group();
    expect(markup).toContain("14 titles · 3 to download");
    expect(markup).toContain("Requested by Jordan");
    expect(markup).toContain("For the film club.");
  });

  it("labels the negative action for the kind of decision", () => {
    expect(group()).toContain(">Deny<");
    expect(group({ denyLabel: "Dismiss" })).toContain(">Dismiss<");
  });

  it("swaps the row actions for a checkbox in Select mode, with the ticked row checked", () => {
    const markup = group({ selected: ["a"], selecting: true });
    expect(markup.match(/role="checkbox"/g)).toHaveLength(2);
    expect(markup.match(/aria-checked="true"/g)).toHaveLength(1);
    expect(markup).not.toContain(">Approve<");
  });

  it("leaves a lone row without Select", () => {
    expect(group({ rows: rows.slice(0, 1) })).not.toContain(">Select<");
  });
});

describe("review state", () => {
  const apply = (...actions: Parameters<typeof reviewReducer>[1][]) =>
    actions.reduce(reviewReducer, initialReview);

  it("raises the bulk sheet on the first tick and lowers it when the last is cleared", () => {
    const ticked = apply(
      { group: "requests", type: "toggle-selecting" },
      {
        group: "requests",
        id: "a",
        on: true,
        type: "toggle-selected",
      },
    );
    expect(ticked.sheet).toEqual({ group: "requests", kind: "bulk" });
    expect(
      reviewReducer(ticked, { group: "requests", id: "a", on: false, type: "toggle-selected" }).sheet,
    ).toBeUndefined();
  });

  it("keeps each group's selection to itself", () => {
    const state = apply(
      { group: "requests", id: "a", on: true, type: "toggle-selected" },
      { group: "filler", id: "x", on: true, type: "toggle-selected" },
    );
    expect(state.selected).toEqual({ filler: ["x"], requests: ["a"] });
    expect(state.sheet).toEqual({ group: "filler", kind: "bulk" });
  });

  it("clears Select and the selection when a group's mode is toggled", () => {
    const state = apply(
      { group: "requests", type: "toggle-selecting" },
      { group: "requests", id: "a", on: true, type: "toggle-selected" },
      { group: "requests", type: "toggle-selecting" },
    );
    expect(state.selecting.requests).toBe(false);
    expect(state.selected.requests).toEqual([]);
    expect(state.sheet).toBeUndefined();
  });

  it("shows a result and ends Select for that group only", () => {
    const rowsOut = [{ id: "a", ok: true, title: "Sunday mornings" }];
    const state = apply(
      { group: "requests", type: "toggle-selecting" },
      { group: "filler", type: "toggle-selecting" },
      { group: "requests", id: "a", on: true, type: "toggle-selected" },
      { group: "requests", rows: rowsOut, type: "show-result" },
    );
    expect(state.sheet).toEqual({ group: "requests", kind: "result", rows: rowsOut });
    expect(state.selecting).toEqual({ filler: true, requests: false });
    expect(state.selected.requests).toEqual([]);
  });

  it("opens deny and edit sheets for one row and closes them", () => {
    expect(apply({ group: "filler", id: "x", type: "open-deny" }).sheet).toEqual({
      group: "filler",
      id: "x",
      kind: "deny",
    });
    expect(apply({ id: "a", type: "open-edit" }).sheet).toEqual({ id: "a", kind: "edit" });
    expect(apply({ id: "a", type: "open-edit" }, { type: "close-sheet" }).sheet).toBeUndefined();
  });
});

describe("review sheet", () => {
  const sheet = (review: Partial<ReviewState>, over: Partial<ReviewSheetProps> = {}) =>
    render(
      <ReviewSheet
        busy={false}
        onApproveEdited={noop}
        onApproveSelected={noop}
        onClose={noop}
        onDeny={noop}
        proposals={approvals}
        pulls={fillerPulls}
        review={{ ...initialReview, ...review }}
        {...over}
      />,
    );

  it("draws nothing without a sheet", () => {
    expect(sheet({})).not.toContain("Scope");
  });

  it("confirms a bulk approve with its count, its group and the one-group scope", () => {
    const markup = sheet({
      selected: { filler: [], requests: ["approval-grace", "approval-mary"] },
      sheet: { group: "requests", kind: "bulk" },
    });
    expect(markup).toContain("2 selected in Channel requests");
    expect(markup).toContain(
      "Scope: this action approves only this group. Select items in another group separately.",
    );
    expect(markup).toContain(">Approve 2<");
    expect(markup).toContain(">Cancel<");
    expect(markup).toContain('aria-label="Bulk approve"');
  });

  it("counts only the rows still waiting when a ticked row was decided elsewhere", () => {
    const markup = sheet({
      selected: { filler: [], requests: ["approval-grace", "gone"] },
      sheet: { group: "requests", kind: "bulk" },
    });
    expect(markup).toContain("1 selected in Channel requests");
  });

  it("holds the approve button while a decision is in flight", () => {
    const markup = sheet(
      { selected: { filler: ["pull-bumpers"], requests: [] }, sheet: { group: "filler", kind: "bulk" } },
      { busy: true },
    );
    expect(markup).toContain("1 selected in Filler downloads");
    expect(markup).toContain('aria-disabled="true"');
  });

  it("reports a partial failure id by id, in BulkApproveResult's shape", () => {
    const markup = sheet({
      sheet: {
        group: "requests",
        kind: "result",
        rows: [
          { id: "a", ok: true, title: "Sunday mornings" },
          { id: "b", ok: true, title: "Jazz and rain" },
          { error: "Already decided by another admin", id: "c", ok: false, title: "Mountain documentaries" },
        ],
      },
    });
    expect(markup).toContain("Approved 2 of 3 · 1 couldn&#x27;t be approved");
    expect(markup).toContain("✓ Sunday mornings");
    expect(markup).toContain("✕ Mountain documentaries — Already decided by another admin");
    expect(markup).toContain(">Done<");
  });

  it("says a clean result plainly", () => {
    const markup = sheet({
      sheet: { group: "filler", kind: "result", rows: [{ id: "a", ok: true, title: "Stingers" }] },
    });
    expect(markup).toContain("Approved 1 of 1");
    expect(markup).not.toContain("couldn");
  });

  it("denies a channel request with an optional note to the requester", () => {
    const markup = sheet({ sheet: { group: "requests", id: "approval-grace", kind: "deny" } });
    expect(markup).toContain("Deny &quot;Black-and-white thrillers&quot;?");
    expect(markup).toContain("Optional note to the requester");
    expect(markup).toContain('aria-label="Deny request"');
  });

  it("dismisses a filler download without a note, since it has no requester", () => {
    const markup = sheet({ sheet: { group: "filler", id: "pull-bumpers", kind: "deny" } });
    expect(markup).toContain("Dismiss &quot;Retro game-show bumpers&quot;?");
    expect(markup).not.toContain("Optional note");
  });

  it("edits a request by unticking titles, approving on the same call", () => {
    const markup = sheet({ sheet: { id: "approval-grace", kind: "edit" } });
    expect(markup).toContain("The Lighthouse Ledger (1998)");
    expect(markup).toContain("Marmalade Street (2003)");
    expect(markup).toContain("Harbour Lights (2011)");
    expect(markup.match(/aria-checked="true"/g)).toHaveLength(3);
    expect(markup).toContain(">Approve with changes<");
  });

  it("draws nothing for a row that is gone", () => {
    expect(sheet({ sheet: { id: "gone", kind: "edit" } })).not.toContain("Approve with changes");
    expect(sheet({ sheet: { group: "filler", id: "gone", kind: "deny" } })).not.toContain("Dismiss");
  });
});

describe("request detail", () => {
  const detail = (key: keyof typeof journeys | undefined, over = {}) =>
    render(
      <RequestDetail
        entry={
          key ? requestsSnapshot().entries.find((e) => e.journey.jobId === journeys[key].jobId) : undefined
        }
        onBack={noop}
        onFix={noop}
        onOpenChannel={noop}
        titles={requestFixtures.titles}
        {...over}
      />,
    );

  it("shows the live stage and pick count while generating", () => {
    const markup = detail("generating");
    expect(markup).toContain("Choosing titles…");
    expect(markup).toContain("2 of about 8 picked");
  });

  it("explains a failure and offers the fix", () => {
    const markup = detail("failed");
    expect(markup).toContain("Only 2 titles matched that description.");
    expect(markup).toContain("A channel needs at least 5 titles.");
    expect(markup).toContain(">Edit and try again<");
  });

  it("says a waiting request is waiting for an admin", () => {
    const markup = detail("awaitingApproval");
    expect(markup).toContain("Waiting for an admin to approve this request.");
    expect(markup).toContain("Marmalade Street (2003)");
  });

  it("lists only the titles still on their way, each with its state", () => {
    const markup = detail("downloading");
    expect(markup).toContain("Approved by Ada Lovelace");
    expect(markup).toContain("Titles being added");
    expect(markup).toContain(">Downloading<");
    expect(markup).toContain(">Waiting<");
  });

  it("opens the channel a finished request became", () => {
    const markup = detail("live");
    expect(markup).toContain(">Open channel<");
    expect(markup).toContain("All titles are on the channel.");
  });

  it("gives a declined request its reason", () => {
    expect(detail("denied")).toContain("Not approved: That&#x27;s more than the library can hold.");
  });

  it("says plainly when the request isn't in the list", () => {
    expect(detail(undefined)).toContain("That request isn&#x27;t here");
  });
});

describe("request a channel", () => {
  const form = (over: Partial<RequestChannelProps> = {}) =>
    render(
      <RequestChannel
        brief={{ kind: "idle" }}
        ideas={ideas}
        ideasStatus="ready"
        nowMs={Date.UTC(2026, 9, 3)}
        onBack={noop}
        onHideIdea={noop}
        onRequestIdea={noop}
        onRetryIdeas={noop}
        onSubmitBrief={noop}
        onUndoHide={noop}
        viewer="member"
        {...over}
      />,
    );

  it("offers idea cards and a typed brief, in Web's words", () => {
    const markup = form();
    expect(markup).toContain("Request a channel");
    expect(markup).toContain("You&#x27;ll choose the final titles before anything is created.");
    expect(markup).toContain("Fireside Mysteries");
    expect(markup).toContain("14 mystery movies, none on a channel yet");
    expect(markup).toContain("Request channel: Fireside Mysteries");
    expect(markup).toContain("Not for me: Fireside Mysteries");
    expect(markup).toContain("Describe the channel");
    expect(markup).toContain(">Suggest titles<");
    expect(markup).toContain("90s Saturday Morning Cartoons");
  });

  it("marks an idea that is already requested instead of offering it again", () => {
    const markup = form();
    expect(markup).toContain(">Waiting for an admin<");
    expect(markup).not.toContain("Request channel: Couch Comedies");
  });

  it("pages through ideas three at a time", () => {
    const many = Array.from({ length: 5 }, (_, i) => ({
      ...(ideas[0] as never as object),
      id: `i${i}`,
      name: `Idea ${i}`,
    }));
    const markup = form({ ideas: many as never });
    expect(markup).toContain("Idea 2");
    expect(markup).not.toContain("Idea 3");
    expect(markup).toContain(">Different ideas<");
  });

  it("says the ideas are loading, failed, or used up", () => {
    expect(form({ ideasStatus: "loading" })).toContain("Finding ideas from your library");
    expect(form({ ideasStatus: "error" })).toContain("Couldn&#x27;t load channel ideas");
    expect(form({ ideas: [] })).toContain("That&#x27;s every idea for now.");
  });

  it("offers Undo for a hidden idea", () => {
    const markup = form({ hiddenIdea: ideas[0] });
    expect(markup).toContain("Hid Fireside Mysteries");
    expect(markup).toContain(">Undo<");
  });

  it("keeps the draft and names who can fix AI being off", () => {
    const member = form({ brief: { kind: "unavailable", reason: "ai" } });
    expect(member).toContain("Finish AI setup");
    expect(member).toContain("An administrator needs to finish AI setup");
    expect(member).toContain("Your draft is saved.");
    expect(member).toContain("Fireside Mysteries");
    const asAdmin = form({ brief: { kind: "unavailable", reason: "ai" }, viewer: "admin" });
    expect(asAdmin).toContain("Connect an AI service and choose a model");
  });

  it("tells a TMDB gap from an AI gap", () => {
    const markup = form({ brief: { kind: "unavailable", reason: "grounding" } });
    expect(markup).toContain("Connect TMDB to build this channel");
    expect(markup).toContain("an administrator needs to connect TMDB");
  });

  it("shows another refusal and disables the form while sending", () => {
    expect(form({ brief: { kind: "failed", message: "You have too many pending requests" } })).toContain(
      "You have too many pending requests",
    );
    expect(form({ brief: { kind: "sending" } })).toContain(">Sending…<");
  });

  it("brings a failed request's text back to the form for editing", () => {
    expect(form({ initialIntent: { description: "Silent films scored by modern bands" } })).toContain(
      "Silent films scored by modern bands",
    );
  });
});

describe("requests journey", () => {
  const controller = (snapshot: RequestsSnapshot): RequestsController => ({
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
    subscribe: () => () => undefined,
    undoHideIdea: vi.fn(),
  });

  it("renders the list under a header with the one primary action", () => {
    const markup = render(
      <RequestsJourney controller={controller(requestsSnapshot())} onOpenChannel={noop} />,
    );
    // "Request a channel" is the app bar's icon button on both phones, as the approved mock draws it.
    expect(markup).toContain('aria-label="Request a channel"');
    expect(markup).toContain(">+<");
    expect(markup).toContain("In progress (3)");
  });

  it("docks the host's navigation under the screen", () => {
    const markup = render(
      <RequestsJourney
        controller={controller(requestsSnapshot())}
        footer={<p>host tab bar</p>}
        onOpenChannel={noop}
      />,
    );
    expect(markup).toContain("host tab bar");
  });

  it("shows a refused action once, above the list", () => {
    const markup = render(
      <RequestsJourney
        controller={controller(requestsSnapshot({ notice: "Admins only" }))}
        onOpenChannel={noop}
      />,
    );
    expect(markup).toContain("Admins only");
    expect(markup).toContain('role="alert"');
    expect(markup).toContain(">Dismiss<");
  });
});
