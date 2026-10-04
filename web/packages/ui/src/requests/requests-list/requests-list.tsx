import type { ProposalDTO, PullDTO } from "@loomarr/core/requests";
import {
  requestFailureHint,
  requestFixLabel,
  requestsInTab,
  requestsNeedsYouCount,
} from "@loomarr/core/requests";
import { Action, SegmentedControl, Skeleton, Surface, Text } from "@loomarr/design-system";

import { StatePanel } from "../../state-panel";
import { ApprovalGroup, type ApprovalRow } from "../approval-group";
import { RequestCard } from "../request-card";
import type { RequestsListProps } from "./requests-list.type";

// The phone's Requests list, in Web's words and order: Needs you, In progress, Done. Needs you holds
// an admin's two groups first, Channel requests and Filler downloads (they block other people), and
// everyone's own failures after, each with its reason and one way to fix it. Members never see the
// groups: deciding is admin-only.
const tabEmpty = {
  "in-progress": {
    description: "Requests that are still being generated, approved or downloaded appear here.",
    title: "Nothing in progress",
  },
  done: {
    description: "Requests that are on their channel, or were declined, appear here.",
    title: "Nothing finished yet",
  },
  "needs-you": {
    description: "Approvals waiting on you, and requests that couldn't be built, appear here.",
    title: "Nothing needs you",
  },
} as const;

// The count shows only when there is something to count, as the approved mock's tabs do.
const counted = (count: number) => (count > 0 ? { count } : {});

const Section = ({ children, title }: { children: React.ReactNode; title: string }) => (
  <Surface backgroundColor="$transparent" borderWidth={0} gap="$control" role="group">
    <Text density="touch" textRole="headline">
      {title}
    </Text>
    {children}
  </Surface>
);

const plural = (count: number, noun: string) => `${count} ${noun}${count === 1 ? "" : "s"}`;

const proposalRow = ({ createdByName, id, note, proposal }: ProposalDTO): ApprovalRow => ({
  id,
  meta: [
    plural(proposal.lineup.length, "title"),
    proposal.acquisitions.length > 0 ? `${proposal.acquisitions.length} to download` : "all in your library",
  ].join(" · "),
  note,
  requestedBy: createdByName,
  title: proposal.intent.description,
});

const pullRow = ({ estimateClips, id, proposedBy, sources, title }: PullDTO): ApprovalRow => ({
  id,
  meta: [plural(estimateClips, "clip"), plural(sources.length, "source")].join(" · "),
  requestedBy: proposedBy,
  title,
});

const RequestsBody = ({
  onApprove,
  onApproveFiller,
  onFix,
  onOpen,
  onOpenChannel,
  onRequestChannel,
  onReview,
  onRetry,
  nowMs,
  onTabChange,
  review,
  snapshot,
  tab,
}: RequestsListProps) => {
  const ask = { label: "Request a channel", onPress: onRequestChannel };

  // Three skeleton rows while the first read is in flight, as the approved mock draws it.
  if (snapshot.status === "loading")
    return (
      <Surface
        accessibilityLabel="Loading your requests"
        aria-busy
        backgroundColor="$transparent"
        borderWidth={0}
        gap="$inline"
        role="status"
      >
        {[0, 1, 2].map((row) => (
          <Surface gap="$inline" key={row} level="raised" padding="$control">
            <Skeleton shape="line" width="40%" />
            <Skeleton shape="line" width="85%" />
            <Skeleton shape="line" width="60%" />
          </Surface>
        ))}
      </Surface>
    );
  if (snapshot.status === "error")
    return (
      <StatePanel
        action={{ label: "Try again", onPress: onRetry }}
        density="touch"
        description="Check your connection and try again."
        kind="error"
        title="Couldn't load your requests."
      />
    );
  const isAdmin = snapshot.role === "admin";
  const approvals = isAdmin ? snapshot.approvals : [];
  const pulls = isAdmin ? snapshot.fillerPulls : [];
  if (snapshot.entries.length === 0 && approvals.length === 0 && pulls.length === 0)
    return (
      <StatePanel
        action={ask}
        density="touch"
        description="Describe a channel and Loomarr will build it. You'll follow it here, from request to your guide."
        kind="empty"
        title="You haven't requested a channel yet"
      />
    );

  const inProgress = requestsInTab(snapshot, "in-progress");
  const done = requestsInTab(snapshot, "done");
  // Needs you is an admin's inbox; a member gets two segments until one of their requests couldn't be
  // built, and never loses the segment they are standing on.
  const needsYouCount = requestsNeedsYouCount(snapshot);
  const showNeedsYou = isAdmin || needsYouCount > 0 || tab === "needs-you";
  const needsYouSegment = { ...counted(needsYouCount), label: "Needs you", value: "needs-you" } as const;
  const failed = requestsInTab(snapshot, "needs-you");
  const find = <Item extends { id: string }>(items: readonly Item[], id: string) =>
    items.find((item) => item.id === id);

  let content: React.ReactNode;
  if (tab === "needs-you") {
    content =
      approvals.length === 0 && pulls.length === 0 && failed.length === 0 ? (
        <StatePanel action={ask} density="touch" kind="empty" {...tabEmpty["needs-you"]} />
      ) : (
        <>
          {approvals.length > 0 ? (
            <ApprovalGroup
              deciding={snapshot.deciding}
              onApprove={(id) => {
                const proposal = find(approvals, id);
                if (proposal) onApprove(proposal);
              }}
              onDeny={(id) => onReview({ group: "requests", id, type: "open-deny" })}
              onEdit={(id) => onReview({ id, type: "open-edit" })}
              onToggleSelected={(id, on) => onReview({ group: "requests", id, on, type: "toggle-selected" })}
              onToggleSelecting={() => onReview({ group: "requests", type: "toggle-selecting" })}
              rows={approvals.map(proposalRow)}
              selected={review.selected.requests}
              selecting={review.selecting.requests}
              title="Channel requests"
            />
          ) : null}
          {pulls.length > 0 ? (
            <ApprovalGroup
              deciding={snapshot.deciding}
              denyLabel="Dismiss"
              onApprove={(id) => {
                const pull = find(pulls, id);
                if (pull) onApproveFiller(pull);
              }}
              onDeny={(id) => onReview({ group: "filler", id, type: "open-deny" })}
              onToggleSelected={(id, on) => onReview({ group: "filler", id, on, type: "toggle-selected" })}
              onToggleSelecting={() => onReview({ group: "filler", type: "toggle-selecting" })}
              rows={pulls.map(pullRow)}
              selected={review.selected.filler}
              selecting={review.selecting.filler}
              title="Filler downloads"
            />
          ) : null}
          {failed.length > 0 ? (
            <Section title="Couldn't be built">
              {failed.map((entry) => {
                const fix = requestFixLabel(entry.journey);
                return (
                  <RequestCard
                    action={fix ? { label: fix, onPress: () => onFix(entry) } : undefined}
                    entry={entry}
                    nowMs={nowMs}
                    hint={requestFailureHint(entry.status.detail, entry.journey.failure?.guidance)}
                    key={entry.journey.jobId}
                    onOpen={() => onOpen(entry.journey.jobId)}
                  />
                );
              })}
            </Section>
          ) : null}
        </>
      );
  } else {
    const entries = tab === "done" ? done : inProgress;
    content =
      entries.length === 0 ? (
        <StatePanel action={ask} density="touch" kind="empty" {...tabEmpty[tab]} />
      ) : (
        entries.map((entry) => {
          const channel = tab === "done" ? entry.journey.channel : undefined;
          return (
            <RequestCard
              action={
                channel ? { label: "Open channel", onPress: () => onOpenChannel(channel.id) } : undefined
              }
              entry={entry}
              key={entry.journey.jobId}
              nowMs={nowMs}
              onOpen={() => onOpen(entry.journey.jobId)}
            />
          );
        })
      );
  }

  return (
    <Surface backgroundColor="$transparent" borderWidth={0} gap="$control">
      {/* iPhone's segmented control: full width, so three counted labels never overflow the phone. */}
      <SegmentedControl
        accessibilityLabel="Requests sections"
        onValueChange={onTabChange}
        options={[
          ...(showNeedsYou ? [needsYouSegment] : []),
          { ...counted(inProgress.length), label: "In progress", value: "in-progress" },
          { ...counted(done.length), label: "Done", value: "done" },
        ]}
        value={tab}
      />
      {content}
    </Surface>
  );
};

// The list screen owns its large title and the app bar's "Request a channel" icon button, above the
// segments in every state (loading, error, empty and ready), so no host or story has to add them. The
// approved mock puts the primary action in the app bar on both phones (Android has no FAB).
const RequestsList = (props: RequestsListProps) => (
  <Surface backgroundColor="$transparent" borderWidth={0} gap="$control">
    <Surface
      alignItems="center"
      backgroundColor="$transparent"
      borderWidth={0}
      flexDirection="row"
      justifyContent="space-between"
    >
      <Text accessibilityRole="header" density="touch" textRole="display">
        Requests
      </Text>
      <Action
        accessibilityLabel="Request a channel"
        accessibilityRole="button"
        density="touch"
        onPress={props.onRequestChannel}
        style={{ minWidth: 48 }}
        tone="primary"
      >
        +
      </Action>
    </Surface>
    <RequestsBody {...props} />
  </Surface>
);

export { RequestsList };
