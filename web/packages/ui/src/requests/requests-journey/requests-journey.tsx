import {
  type ApprovalEditDTO,
  type Intent,
  type ProposalDTO,
  type PullDTO,
  type RequestsController,
  type RequestsSnapshot,
  type RequestTab,
  requestsInTab,
  requestsNeedsYouCount,
} from "@loomarr/core/requests";
import { Action, PHONE_GUTTER, Screen, ScrollFrame, Surface, Text } from "@loomarr/design-system";
import { useEffect, useReducer, useState, useSyncExternalStore } from "react";
import { RefreshControl } from "react-native";
import type { BriefState } from "../request-channel";
import { RequestChannel } from "../request-channel";
import { RequestDetail } from "../request-detail";
import { RequestsList } from "../requests-list";
import { initialReview, type ReviewGroup, reviewReducer } from "../review";
import { ReviewSheet } from "../review-sheet";
import type { RequestsJourneyProps } from "./requests-journey.type";

type Route =
  | { view: "list" }
  | { jobId: string; view: "detail" }
  | { initialIntent?: Intent; view: "request" };

// The section a person last picked outlives this screen (the host unmounts it when they visit another
// tab), so it is kept per controller: the host creates one for the app's lifetime, and a new one,
// as after pairing again, starts over.
const lastPickedTab = new WeakMap<RequestsController, RequestTab>();

// Opens on the first section with something in it, in the order they're drawn: Needs you, In progress, Done.
const firstFilledTab = (snapshot: RequestsSnapshot): RequestTab =>
  requestsNeedsYouCount(snapshot) > 0
    ? "needs-you"
    : requestsInTab(snapshot, "in-progress").length === 0 && requestsInTab(snapshot, "done").length > 0
      ? "done"
      : "in-progress";

type Banner = {
  action?: { label: string; onPress: () => void };
  message: string;
  tone: "danger" | "success";
};

// The phone's Requests, end to end: the list, one request, and the "Request a channel" form. It reads
// one RequestsController (the host owns its lifetime and disposes it), so the badge, the tab counts and
// every list agree. Admin controls appear only for the role `/v1/auth/me` reports, as on Web.
const RequestsJourney = ({
  channels,
  controller,
  footer,
  headerAction,
  nowMs,
  onOpenChannel,
}: RequestsJourneyProps) => {
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const [route, setRoute] = useState<Route>({ view: "list" });
  // A list already read when the screen opens (the host loads it with the app) draws on its section at once.
  const [tab, setTab] = useState<RequestTab>(
    () => lastPickedTab.get(controller) ?? firstFilledTab(controller.getSnapshot()),
  );
  const [refreshing, setRefreshing] = useState(false);
  const [brief, setBrief] = useState<BriefState>({ kind: "idle" });
  const [approved, setApproved] = useState<Banner>();
  const [review, dispatchReview] = useReducer(reviewReducer, initialReview);

  useEffect(() => {
    void controller.refresh();
  }, [controller]);
  // An admin's form has no ideas to show (RequestChannel), so it reads none.
  const requestingIdeas = route.view === "request" && snapshot.role !== "admin";
  useEffect(() => {
    if (requestingIdeas) void controller.refreshIdeas();
  }, [controller, requestingIdeas]);

  // Land once on the section the person last picked, else the first with something in it. Later
  // refreshes never move them off the section they are reading.
  const [landed, setLanded] = useState(
    () => lastPickedTab.has(controller) || controller.getSnapshot().status === "ready",
  );
  useEffect(() => {
    if (snapshot.status === "ready" && !landed) {
      setLanded(true);
      setTab(firstFilledTab(snapshot));
    }
  }, [landed, snapshot]);
  const pickTab = (picked: RequestTab) => {
    lastPickedTab.set(controller, picked);
    setTab(picked);
  };
  // Pull-to-refresh spins until the list has been read again, whatever the read's outcome.
  const refreshList = async () => {
    setRefreshing(true);
    try {
      await controller.refresh();
    } finally {
      setRefreshing(false);
    }
  };

  const banner: Banner | undefined = snapshot.notice
    ? { message: snapshot.notice, tone: "danger" }
    : approved;
  const dismissBanner = () => {
    controller.dismissNotice();
    setApproved(undefined);
  };

  // Back on the list, on the section the person was reading. A refusal the form showed (an idea that
  // couldn't be requested) belongs to the form, so it goes with it rather than greeting the list.
  const goList = () => {
    if (route.view === "request") controller.dismissNotice();
    setBrief({ kind: "idle" });
    setRoute({ view: "list" });
  };

  const approve = async (proposal: ProposalDTO, edit?: ApprovalEditDTO) => {
    const result = await controller.approve(proposal.id, edit);
    dispatchReview({ type: "close-sheet" });
    if (result.kind === "done")
      setApproved({
        action: result.channelId
          ? { label: "Open channel", onPress: () => onOpenChannel(result.channelId ?? "") }
          : undefined,
        message: "Approved. The channel is being built and its titles are on their way.",
        tone: "success",
      });
  };
  const approveFiller = async (pull: PullDTO) => {
    const result = await controller.approveFillerPull(pull.id);
    if (result.kind === "done")
      setApproved({ message: `Approved "${pull.title}". Its clips are on their way.`, tone: "success" });
  };
  // A bulk approve names every id's outcome, so the result sheet opens whether or not all went through;
  // only a call that could not run at all closes the sheet and leaves its sentence in the banner.
  const approveSelected = async (group: ReviewGroup) => {
    const rows =
      group === "requests"
        ? snapshot.approvals.map((proposal) => ({
            id: proposal.id,
            title: proposal.proposal.intent.description,
          }))
        : snapshot.fillerPulls.map((pull) => ({ id: pull.id, title: pull.title }));
    const chosen = rows.filter((row) => review.selected[group].includes(row.id));
    const ids = chosen.map((row) => row.id);
    const result = await (group === "requests"
      ? controller.approveMany(ids)
      : controller.approveFillerPulls(ids));
    if (result.kind === "failed") return dispatchReview({ type: "close-sheet" });
    dispatchReview({
      group,
      rows: chosen.map(({ id, title }) => {
        const outcome = result.results.find((candidate) => candidate.id === id);
        return { ...(outcome?.error ? { error: outcome.error } : {}), id, ok: outcome?.ok ?? false, title };
      }),
      type: "show-result",
    });
  };
  const deny = async (group: ReviewGroup, id: string, reason?: string) => {
    const result =
      group === "requests" ? await controller.deny(id, reason) : await controller.dismissFillerPull(id);
    dispatchReview({ type: "close-sheet" });
    if (result.kind === "done")
      setApproved({
        message:
          group === "requests"
            ? "Denied. The requester will see your reason."
            : "Dismissed. Nothing will be downloaded.",
        tone: "success",
      });
  };

  let body: React.ReactNode;
  if (route.view === "detail") {
    body = (
      <RequestDetail
        entry={snapshot.entries.find((entry) => entry.journey.jobId === route.jobId)}
        onBack={() => goList()}
        onFix={(entry) => setRoute({ initialIntent: entry.journey.intent, view: "request" })}
        onOpenChannel={onOpenChannel}
        titles={snapshot.titles}
      />
    );
  } else if (route.view === "request") {
    body = (
      <RequestChannel
        brief={brief}
        hiddenIdea={snapshot.hiddenIdea}
        ideas={snapshot.ideas}
        ideasStatus={snapshot.ideasStatus}
        initialIntent={route.initialIntent}
        nowMs={nowMs ?? Date.now()}
        onBack={() => goList()}
        onHideIdea={(id) => void controller.hideIdea(id)}
        onRequestIdea={(id) => {
          void controller.requestIdea(id);
        }}
        onRetryIdeas={() => void controller.refreshIdeas()}
        onSubmitBrief={async (intent) => {
          setBrief({ kind: "sending" });
          const result = await controller.submitBrief(intent);
          if (result.kind === "started") {
            controller.dismissNotice();
            setBrief({ kind: "idle" });
            setRoute({ jobId: result.jobId, view: "detail" });
          } else if (result.kind === "unavailable") setBrief(result);
          else setBrief(result);
        }}
        onUndoHide={() => void controller.undoHideIdea()}
        requestingIdeaId={snapshot.requestingIdeaId}
        viewer={snapshot.role}
      />
    );
  } else {
    body = (
      <ScrollFrame
        density="touch"
        refreshControl={<RefreshControl onRefresh={() => void refreshList()} refreshing={refreshing} />}
      >
        <RequestsList
          channels={channels}
          headerAction={headerAction}
          nowMs={nowMs}
          onApprove={(proposal) => void approve(proposal)}
          onApproveFiller={(pull) => void approveFiller(pull)}
          onFix={(entry) => setRoute({ initialIntent: entry.journey.intent, view: "request" })}
          onOpen={(jobId) => setRoute({ jobId, view: "detail" })}
          onOpenChannel={onOpenChannel}
          onRequestChannel={() => setRoute({ view: "request" })}
          onReview={dispatchReview}
          onRetry={() => void controller.refresh()}
          onTabChange={(picked) => {
            dispatchReview({ type: "close-sheet" });
            pickTab(picked);
          }}
          review={review}
          snapshot={snapshot}
          tab={tab}
        />
      </ScrollFrame>
    );
  }

  // The review sheet docks directly above the host's tab bar, and only on the list it belongs to.
  const sheet =
    route.view === "list" ? (
      <ReviewSheet
        busy={snapshot.deciding.length > 0}
        onApproveEdited={(proposal, edit) => void approve(proposal, edit)}
        onApproveSelected={(group) => void approveSelected(group)}
        onClose={() => dispatchReview({ type: "close-sheet" })}
        onDeny={(group, id, reason) => void deny(group, id, reason)}
        proposals={snapshot.approvals}
        pulls={snapshot.fillerPulls}
        review={review}
      />
    ) : null;

  // Flush, with the phone gutter on the sides only, so the large title sits where the Guide's does:
  // 6 pt under the status bar, not under a top gutter as well.
  return (
    <Screen
      density="touch"
      flush
      footer={
        sheet ? (
          <>
            {sheet}
            {footer}
          </>
        ) : (
          footer
        )
      }
    >
      <Surface
        backgroundColor="$transparent"
        borderWidth={0}
        flex={1}
        gap="$control"
        paddingHorizontal={PHONE_GUTTER}
      >
        {banner ? (
          <Surface
            gap="$inline"
            level="raised"
            marginTop="$inline"
            padding="$control"
            role={banner.tone === "danger" ? "alert" : "status"}
          >
            <Text density="touch" textRole="body" tone={banner.tone}>
              {banner.message}
            </Text>
            <Surface backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap="$inline">
              {banner.action ? (
                <Action
                  accessibilityRole="button"
                  density="touch"
                  onPress={banner.action.onPress}
                  style={{ flex: 1 }}
                  tone="secondary"
                >
                  {banner.action.label}
                </Action>
              ) : null}
              <Action
                accessibilityRole="button"
                density="touch"
                onPress={dismissBanner}
                style={{ flex: 1 }}
                tone="secondary"
              >
                Dismiss
              </Action>
            </Surface>
          </Surface>
        ) : null}
        {body}
      </Surface>
    </Screen>
  );
};

export { RequestsJourney };
