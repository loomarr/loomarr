import {
  type ApprovalResult,
  createRequestsController,
  type ProposalDTO,
  type PullDTO,
  type RequestsPort,
  requestsNeedsYouCount,
} from "@loomarr/core";
import { Screen, ScrollFrame, TabBar } from "@loomarr/design-system";
import { requestFixtures, requestsSnapshot } from "@loomarr/fixtures";
import {
  initialReview,
  RequestChannel,
  type RequestChannelProps,
  RequestDetail,
  RequestsJourney,
  RequestsList,
  ReviewSheet,
  type ReviewState,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useEffect, useMemo, useState } from "react";

// Channel Requests on the phone (#1816, decision N2), for maintainer review. Every title, name and
// request here is invented (@loomarr/fixtures); nothing is copied from a design mock.
//
// The list stories drive the real controller over a fake port, so Approve, Deny, Request channel and
// Not for me all respond. The detail, form and tab stories freeze one state of one screen.

type Scenario = "admin" | "empty" | "error" | "loading" | "member";
type JourneyKey = keyof typeof requestFixtures.journeys;
type FormArgs = Partial<Omit<RequestChannelProps, "viewer">>;

const never = () => new Promise<never>(() => undefined);

// Like the approved mock's simulated result: when several are approved at once, the last one is
// refused as already decided, so a partial failure is on screen without hunting for it.
const settleMany = (ids: readonly string[]): { approved: number; results: ApprovalResult[] } => {
  const results = ids.map((id, index) =>
    ids.length > 1 && index === ids.length - 1
      ? { error: "Already decided by another admin", id, ok: false }
      : { id, ok: true },
  );
  return { approved: results.filter((result) => result.ok).length, results };
};

const fakePort = (scenario: Scenario): RequestsPort => {
  let waiting: ProposalDTO[] = [...requestFixtures.approvals];
  let pulls: PullDTO[] = [...requestFixtures.fillerPulls];
  const read = <T,>(value: T): Promise<T> =>
    scenario === "loading"
      ? never()
      : scenario === "error"
        ? Promise.reject(new Error("The server didn't answer."))
        : Promise.resolve(value);
  return {
    approve: async (id) => {
      waiting = waiting.filter((proposal) => proposal.id !== id);
      return { channelId: "channel-new" };
    },
    approveFillerPulls: async (ids) => {
      const outcome = settleMany(ids);
      const done = outcome.results.filter((result) => result.ok).map((result) => result.id);
      pulls = pulls.filter((pull) => !done.includes(pull.id));
      return outcome;
    },
    approveMany: async (ids) => {
      const outcome = settleMany(ids);
      const done = outcome.results.filter((result) => result.ok).map((result) => result.id);
      waiting = waiting.filter((proposal) => !done.includes(proposal.id));
      return outcome;
    },
    deny: async (id) => {
      waiting = waiting.filter((proposal) => proposal.id !== id);
    },
    dismissFillerPull: async (id) => {
      pulls = pulls.filter((pull) => pull.id !== id);
    },
    hideIdea: async () => undefined,
    loadApprovals: () => read(waiting),
    loadFillerPulls: () => read(pulls),
    loadIdeas: () => read([...requestFixtures.ideas]),
    loadJourneys: () => read(scenario === "empty" ? [] : Object.values(requestFixtures.journeys)),
    loadMe: () => read({ role: scenario === "admin" ? "admin" : "member" } as never),
    loadTitles: () => read([...requestFixtures.titles]),
    requestIdea: async () => ({ jobId: "job-new" }),
    submitBrief: async () => ({ jobId: requestFixtures.journeys.generating.jobId }),
    unhideIdea: async () => undefined,
  };
};

// Decided with the approved mock (#1840): option A, Watching · Guide · Requests · Surf, with Web's
// count badge. The shell is not changed here; see the PR for the exact wiring.
const navigation = (badge: number) => (
  <TabBar
    accessibilityLabel="Primary navigation"
    idiom="ios"
    items={[
      { icon: "play", label: "Watching", value: "watching" },
      { icon: "guide", label: "Guide", value: "guide" },
      { badge, icon: "requests", label: "Requests", value: "requests" },
      { icon: "channels", label: "Surf", value: "surf" },
    ]}
    onSelect={() => undefined}
    selected="requests"
  />
);

const NOW = Date.UTC(2026, 9, 3, 12);

const Journey = ({ scenario }: { scenario: Scenario }) => {
  const controller = useMemo(() => createRequestsController({ port: fakePort(scenario) }), [scenario]);
  useEffect(() => () => controller.dispose(), [controller]);
  const [badge, setBadge] = useState(0);
  useEffect(() => {
    const read = () => setBadge(requestsNeedsYouCount(controller.getSnapshot()));
    read();
    return controller.subscribe(read);
  }, [controller]);
  return (
    <RequestsJourney
      controller={controller}
      footer={navigation(badge)}
      nowMs={NOW}
      onOpenChannel={() => undefined}
    />
  );
};

type WorkshopProps = {
  /** Badge on the Requests tab, for the tab-position stories. */
  badge?: number;
  /** A frozen request-form state. */
  form?: FormArgs;
  formRole?: "admin" | "member";
  /** A frozen request detail; `missing` is a request that isn't in the list. */
  journey?: JourneyKey | "missing";
  /** A frozen admin Needs you: the review state (Select mode, the docked sheet) over these waiting items. */
  review?: Partial<ReviewState>;
  reviewWaiting?: { approvals?: boolean; fillerPulls?: boolean };
  scenario?: Scenario;
  screen?: "detail" | "form" | "journey" | "review" | "tabs";
};

const noop = () => undefined;

// A frozen admin Needs you, built from the same RequestsList and ReviewSheet the journey composes, with
// the sheet docked above the tab bar as the host places it.
const ReviewFrame = ({
  review,
  waiting,
}: {
  review: ReviewState;
  waiting: WorkshopProps["reviewWaiting"];
}) => {
  const approvals = waiting?.approvals === false ? [] : requestFixtures.approvals;
  const fillerPulls = waiting?.fillerPulls === false ? [] : requestFixtures.fillerPulls;
  const needsYou = requestsSnapshot({ approvals, fillerPulls, role: "admin" });
  return (
    <Screen
      density="touch"
      footer={
        <>
          <ReviewSheet
            busy={false}
            onApproveEdited={noop}
            onApproveSelected={noop}
            onClose={noop}
            onDeny={noop}
            proposals={approvals}
            pulls={fillerPulls}
            review={review}
          />
          {navigation(requestsNeedsYouCount(needsYou))}
        </>
      }
      gap="$control"
    >
      <ScrollFrame density="touch">
        <RequestsList
          nowMs={NOW}
          onApprove={noop}
          onApproveFiller={noop}
          onFix={noop}
          onOpen={noop}
          onOpenChannel={noop}
          onRequestChannel={noop}
          onReview={noop}
          onRetry={noop}
          onTabChange={noop}
          review={review}
          snapshot={needsYou}
          tab="needs-you"
        />
      </ScrollFrame>
    </Screen>
  );
};

const RequestsWorkshop = ({
  badge = 1,
  form,
  formRole = "member",
  journey,
  review,
  reviewWaiting,
  scenario = "member",
  screen = "journey",
}: WorkshopProps) => {
  if (screen === "journey") return <Journey scenario={scenario} />;
  if (screen === "review")
    return <ReviewFrame review={{ ...initialReview, ...review }} waiting={reviewWaiting} />;
  return (
    <Screen density="touch" footer={navigation(badge)} gap="$control">
      {screen === "detail" ? (
        <RequestDetail
          entry={
            journey && journey !== "missing"
              ? requestsSnapshot().entries.find(
                  (entry) => entry.journey.jobId === requestFixtures.journeys[journey].jobId,
                )
              : undefined
          }
          onBack={() => undefined}
          onFix={() => undefined}
          onOpenChannel={() => undefined}
          titles={requestFixtures.titles}
        />
      ) : null}
      {screen === "form" ? (
        <RequestChannel
          brief={{ kind: "idle" }}
          ideas={requestFixtures.ideas}
          ideasStatus="ready"
          nowMs={NOW}
          onBack={() => undefined}
          onHideIdea={() => undefined}
          onRequestIdea={() => undefined}
          onRetryIdeas={() => undefined}
          onSubmitBrief={() => undefined}
          onUndoHide={() => undefined}
          viewer={formRole}
          {...form}
        />
      ) : null}
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/Requests",
  component: RequestsWorkshop,
  args: { scenario: "member", screen: "journey" },
} satisfies Meta<typeof RequestsWorkshop>;

type Story = StoryObj<typeof meta>;

// The list. A member lands on Needs you when something couldn't be built; an admin also sees approvals.
const MemberList: Story = {};
const AdminNeedsYou: Story = { args: { scenario: "admin" } };
const LoadingList: Story = { args: { scenario: "loading" } };
const ErrorList: Story = { args: { scenario: "error" } };
const NothingRequested: Story = { args: { scenario: "empty" } };
const LightMemberList: Story = { globals: { theme: "light" } };

// Admin Needs you (#1840): Channel requests and Filler downloads are two groups, each with its own
// Select and bulk approve. Every state below is frozen; AdminNeedsYou above is the live one, where
// Select, the sheets, Approve, Deny and Edit all respond and a bulk approve refuses its last item.
const review = (state: Partial<ReviewState>, reviewWaiting?: WorkshopProps["reviewWaiting"]): Story => ({
  args: { review: state, reviewWaiting, screen: "review" },
});
const rowsOut = [
  { id: "approval-grace", ok: true, title: "Black-and-white thrillers" },
  { id: "approval-katherine", ok: true, title: "Mountain documentaries" },
  {
    error: "Already decided by another admin",
    id: "approval-mary",
    ok: false,
    title: "Overnight jazz and rain sounds",
  },
];
const AdminGroups: Story = review({});
const AdminChannelRequestsOnly: Story = review({}, { fillerPulls: false });
const AdminFillerDownloadsOnly: Story = review({}, { approvals: false });
const AdminSelectingRequests: Story = review({ selecting: { filler: false, requests: true } });
const AdminBulkRequests: Story = review({
  selected: { filler: [], requests: ["approval-grace", "approval-katherine"] },
  selecting: { filler: false, requests: true },
  sheet: { group: "requests", kind: "bulk" },
});
const AdminBulkFiller: Story = review({
  selected: { filler: ["pull-bumpers", "pull-stingers"], requests: [] },
  selecting: { filler: true, requests: false },
  sheet: { group: "filler", kind: "bulk" },
});
const AdminBothGroupsSelecting: Story = review({
  selected: { filler: ["pull-stingers"], requests: ["approval-grace"] },
  selecting: { filler: true, requests: true },
  sheet: { group: "filler", kind: "bulk" },
});
const AdminBulkPartialFailure: Story = review({
  sheet: { group: "requests", kind: "result", rows: rowsOut },
});
const AdminBulkAllApproved: Story = review({
  sheet: { group: "requests", kind: "result", rows: rowsOut.slice(0, 2) },
});
const AdminFillerPartialFailure: Story = review({
  sheet: {
    group: "filler",
    kind: "result",
    rows: [
      { id: "pull-bumpers", ok: true, title: "Retro game-show bumpers" },
      {
        error: "Already decided by another admin",
        id: "pull-stingers",
        ok: false,
        title: "Static and tuning-card stingers",
      },
    ],
  },
});
const AdminDenySheet: Story = review({ sheet: { group: "requests", id: "approval-grace", kind: "deny" } });
const AdminDismissSheet: Story = review({ sheet: { group: "filler", id: "pull-bumpers", kind: "deny" } });
const AdminEditSheet: Story = review({ sheet: { id: "approval-grace", kind: "edit" } });
const AdminLightGroups: Story = { ...review({}), globals: { theme: "light" } };

// One request, in each place it can stand.
const detail = (journey: JourneyKey | "missing"): Story => ({ args: { journey, screen: "detail" } });
const DetailGenerating: Story = detail("generating");
const DetailWaiting: Story = detail("awaitingApproval");
const DetailDownloading: Story = detail("downloading");
const DetailDone: Story = detail("live");
const DetailDeclined: Story = detail("denied");
const DetailFailed: Story = detail("failed");
const DetailMissing: Story = detail("missing");

// Request a channel: idea cards and the typed brief, with AI on and off.
const request = (form: FormArgs, formRole: "admin" | "member" = "member"): Story => ({
  args: { form, formRole, screen: "form" },
});
const RequestAChannel: Story = request({});
const RequestIdeasLoading: Story = request({ ideasStatus: "loading" });
const RequestIdeasFailed: Story = request({ ideasStatus: "error" });
const RequestIdeasUsedUp: Story = request({ ideas: [] });
const RequestIdeaHidden: Story = request({ hiddenIdea: requestFixtures.ideas[0] });
const RequestAiOff: Story = request({ brief: { kind: "unavailable", reason: "ai" } });
const RequestAiOffAdmin: Story = request({ brief: { kind: "unavailable", reason: "ai" } }, "admin");
const RequestTmdbOff: Story = request({ brief: { kind: "unavailable", reason: "grounding" } });
const RequestSending: Story = request({ brief: { kind: "sending" } });
const RequestRefused: Story = request({
  brief: { kind: "failed", message: "You have too many pending requests." },
});
const EditAndTryAgain: Story = request({
  initialIntent: { description: requestFixtures.journeys.failed.intent.description },
});

// The tab-position proposal: a fourth tab with Web's pending-count badge.
const TabMember: Story = { args: { badge: 1, screen: "tabs" } };
const TabAdmin: Story = { args: { badge: 3, screen: "tabs" } };

export default meta;
export {
  AdminBothGroupsSelecting,
  AdminBulkAllApproved,
  AdminBulkFiller,
  AdminBulkPartialFailure,
  AdminBulkRequests,
  AdminChannelRequestsOnly,
  AdminDenySheet,
  AdminDismissSheet,
  AdminEditSheet,
  AdminFillerDownloadsOnly,
  AdminFillerPartialFailure,
  AdminGroups,
  AdminLightGroups,
  AdminNeedsYou,
  AdminSelectingRequests,
  DetailDeclined,
  DetailDone,
  DetailDownloading,
  DetailFailed,
  DetailGenerating,
  DetailMissing,
  DetailWaiting,
  EditAndTryAgain,
  ErrorList,
  LightMemberList,
  LoadingList,
  MemberList,
  NothingRequested,
  RequestAChannel,
  RequestAiOff,
  RequestAiOffAdmin,
  RequestIdeaHidden,
  RequestIdeasFailed,
  RequestIdeasLoading,
  RequestIdeasUsedUp,
  RequestRefused,
  RequestSending,
  RequestTmdbOff,
  TabAdmin,
  TabMember,
};
