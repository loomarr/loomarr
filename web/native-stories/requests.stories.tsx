import {
  createRequestsController,
  type ProposalDTO,
  type RequestsPort,
  requestsNeedsYouCount,
} from "@loomarr/core";
import { Screen, TabBar } from "@loomarr/design-system";
import { requestFixtures, requestsSnapshot } from "@loomarr/fixtures";
import { RequestChannel, type RequestChannelProps, RequestDetail, RequestsJourney } from "@loomarr/ui";
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

const fakePort = (scenario: Scenario): RequestsPort => {
  let waiting: ProposalDTO[] = [...requestFixtures.approvals];
  const read = <T,>(value: T): Promise<T> =>
    scenario === "loading"
      ? never()
      : scenario === "error"
        ? Promise.reject(new Error("The server didn't answer."))
        : Promise.resolve(value);
  return {
    approve: async () => {
      waiting = waiting.slice(1);
      return { channelId: "channel-new" };
    },
    approveMany: async (ids) => {
      waiting = waiting.filter((proposal) => !ids.includes(proposal.id));
      return { approved: ids.length, failed: 0 };
    },
    deny: async () => {
      waiting = waiting.slice(1);
    },
    hideIdea: async () => undefined,
    loadApprovals: () => read(waiting),
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
  scenario?: Scenario;
  screen?: "detail" | "form" | "journey" | "tabs";
};

const RequestsWorkshop = ({
  badge = 1,
  form,
  formRole = "member",
  journey,
  scenario = "member",
  screen = "journey",
}: WorkshopProps) => {
  if (screen === "journey") return <Journey scenario={scenario} />;
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
  AdminNeedsYou,
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
