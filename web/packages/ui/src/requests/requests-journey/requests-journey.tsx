import { type Intent, type RequestTab, requestsInTab } from "@loomarr/core/requests";
import { Action, Screen, ScrollFrame, Surface, Text } from "@loomarr/design-system";
import { useEffect, useState, useSyncExternalStore } from "react";
import type { BriefState } from "../request-channel";
import { RequestChannel } from "../request-channel";
import { RequestDetail } from "../request-detail";
import { RequestsList } from "../requests-list";
import type { RequestsJourneyProps } from "./requests-journey.type";

type Route =
  | { tab: RequestTab; view: "list" }
  | { jobId: string; view: "detail" }
  | { initialIntent?: Intent; view: "request" };

type Banner = {
  action?: { label: string; onPress: () => void };
  message: string;
  tone: "danger" | "success";
};

// The phone's Requests, end to end: the list, one request, and the "Request a channel" form. It reads
// one RequestsController (the host owns its lifetime and disposes it), so the badge, the tab counts and
// every list agree. Admin controls appear only for the role `/v1/auth/me` reports, as on Web.
const RequestsJourney = ({ controller, footer, nowMs, onOpenChannel }: RequestsJourneyProps) => {
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const [route, setRoute] = useState<Route>({ tab: "in-progress", view: "list" });
  const [brief, setBrief] = useState<BriefState>({ kind: "idle" });
  const [approved, setApproved] = useState<Banner>();

  useEffect(() => {
    void controller.refresh();
  }, [controller]);
  const requesting = route.view === "request";
  useEffect(() => {
    if (requesting) void controller.refreshIdeas();
  }, [controller, requesting]);

  // Land on Needs you when something is waiting for this person, as Web's badge invites.
  const needsYou =
    requestsInTab(snapshot, "needs-you").length + (snapshot.role === "admin" ? snapshot.approvals.length : 0);
  const [landed, setLanded] = useState(false);
  useEffect(() => {
    if (snapshot.status === "ready" && !landed) {
      setLanded(true);
      if (needsYou > 0) setRoute({ tab: "needs-you", view: "list" });
    }
  }, [landed, needsYou, snapshot.status]);

  const banner: Banner | undefined = snapshot.notice
    ? { message: snapshot.notice, tone: "danger" }
    : approved;
  const dismissBanner = () => {
    controller.dismissNotice();
    setApproved(undefined);
  };

  const goList = (tab: RequestTab = "in-progress") => {
    setBrief({ kind: "idle" });
    setRoute({ tab, view: "list" });
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
      <ScrollFrame density="touch">
        <Text density="touch" textRole="display">
          Requests
        </Text>
        <Text density="touch" textRole="body" tone="secondary">
          Channels you've asked for and where each one stands.
        </Text>
        <Action
          accessibilityRole="button"
          density="touch"
          onPress={() => setRoute({ view: "request" })}
          tone="primary"
        >
          Request a channel
        </Action>
        <RequestsList
          onApprove={async (proposal) => {
            const result = await controller.approve(proposal.id);
            if (result.kind === "done")
              setApproved({
                action: result.channelId
                  ? { label: "Open channel", onPress: () => onOpenChannel(result.channelId ?? "") }
                  : undefined,
                message: "Approved. The channel is being built and its titles are on their way.",
                tone: "success",
              });
          }}
          onApproveSelected={async (ids) => {
            const result = await controller.approveMany(ids);
            if (result.kind === "done")
              setApproved({
                message: `Approved ${ids.length}. Their channels are being built.`,
                tone: "success",
              });
          }}
          onDeny={async (proposal, reason) => {
            const result = await controller.deny(proposal.id, reason);
            if (result.kind === "done")
              setApproved({ message: "Denied. The requester will see your reason.", tone: "success" });
          }}
          onFix={(entry) => setRoute({ initialIntent: entry.journey.intent, view: "request" })}
          onOpen={(jobId) => setRoute({ jobId, view: "detail" })}
          onOpenChannel={onOpenChannel}
          onRequestChannel={() => setRoute({ view: "request" })}
          onRetry={() => void controller.refresh()}
          onTabChange={(tab) => setRoute({ tab, view: "list" })}
          snapshot={snapshot}
          tab={route.view === "list" ? route.tab : "in-progress"}
        />
      </ScrollFrame>
    );
  }

  return (
    <Screen density="touch" footer={footer} gap="$control">
      {banner ? (
        <Surface
          gap="$inline"
          level="raised"
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
    </Screen>
  );
};

export { RequestsJourney };
