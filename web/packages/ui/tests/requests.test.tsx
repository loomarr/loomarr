import type { RequestsController, RequestsSnapshot } from "@loomarr/core/requests";
import { LoomarrProvider } from "@loomarr/design-system";
import { requestFixtures, requestsSnapshot } from "@loomarr/fixtures";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  ApprovalCard,
  RequestChannel,
  type RequestChannelProps,
  RequestDetail,
  RequestsJourney,
  RequestsList,
  type RequestsListProps,
} from "../index";

const render = (node: React.ReactNode) =>
  renderToStaticMarkup(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

const { approvals, ideas, journeys } = requestFixtures;
const admin = (over: Partial<RequestsSnapshot> = {}) =>
  requestsSnapshot({ approvals, role: "admin", ...over });

const noop = vi.fn();
const list = (over: Partial<RequestsListProps>) =>
  render(
    <RequestsList
      onApprove={noop}
      onApproveSelected={noop}
      onDeny={noop}
      onFix={noop}
      onOpen={noop}
      onOpenChannel={noop}
      onRequestChannel={noop}
      onRetry={noop}
      onTabChange={noop}
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
    expect(markup).toContain("Couldn&#x27;t load requests");
    expect(markup).toContain("The server didn&#x27;t answer. Nothing you asked for was lost.");
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
      expect(markup).not.toContain("Waiting for your approval");
      expect(markup).not.toContain(">Approve<");
      expect(markup).toContain("Needs you (1)");
    });

    it("gives an admin the approvals first, counted with their own failures", () => {
      const markup = list({ snapshot: admin(), tab: "needs-you" });
      expect(markup.indexOf("Waiting for your approval")).toBeLessThan(
        markup.indexOf("Couldn&#x27;t be built"),
      );
      expect(markup).toContain("Needs you (3)");
      expect(markup).toContain("Requested by Grace Hopper");
      expect(markup.match(/>Approve</g)).toHaveLength(2);
      expect(markup.match(/>Deny</g)).toHaveLength(2);
      expect(markup).toContain("Approving builds the channel and starts the downloads.");
    });

    it("offers bulk approval only when more than one proposal waits", () => {
      expect(list({ snapshot: admin(), tab: "needs-you" })).toContain(">Approve selected<");
      expect(list({ snapshot: admin({ approvals: approvals.slice(0, 1) }), tab: "needs-you" })).not.toContain(
        "Approve selected",
      );
    });

    it("disables the decision being made", () => {
      const markup = list({ snapshot: admin({ deciding: ["approval-grace"] }), tab: "needs-you" });
      expect(markup).toContain(">Approving…<");
      expect(markup).toContain('aria-disabled="true"');
    });
  });
});

describe("approval card", () => {
  const card = (over = {}) =>
    render(<ApprovalCard onApprove={noop} onDeny={noop} proposal={approvals[0] as never} {...over} />);

  it("counts the titles and what would be downloaded", () => {
    expect(card()).toContain("1 title · 2 to acquire");
    expect(card()).toContain("For the weekend film club.");
  });

  it("says when everything is already in the library", () => {
    expect(card({ proposal: approvals[1] })).toContain("1 title · all in your library");
  });

  it("offers selection as a checkbox when asked", () => {
    expect(card({ onToggleSelected: noop, selected: true })).toContain('role="checkbox"');
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
    approveMany: vi.fn(),
    deny: vi.fn(),
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
    expect(markup).toContain("Channels you&#x27;ve asked for and where each one stands.");
    expect(markup).toContain(">Request a channel<");
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
