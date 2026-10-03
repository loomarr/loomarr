import {
  requestFailureHint,
  requestFixLabel,
  requestsInTab,
  requestsNeedsYouCount,
} from "@loomarr/core/requests";
import { Action, Skeleton, Surface, Tabs, Text } from "@loomarr/design-system";
import { useState } from "react";
import { ScrollView } from "react-native";

import { StatePanel } from "../../state-panel";
import { ApprovalCard } from "../approval-card";
import { RequestCard } from "../request-card";
import type { RequestsListProps } from "./requests-list.type";

// The phone's Requests list, in Web's words and order: Needs you, In progress, Done. Needs you holds
// an admin's approvals first (they block other people) and everyone's own failures after, each with
// its reason and one way to fix it. Members never see approvals: deciding is admin-only.
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
const counted = (label: string, count: number) => (count > 0 ? `${label} (${count})` : label);

const Section = ({ children, title }: { children: React.ReactNode; title: string }) => (
  <Surface backgroundColor="$transparent" borderWidth={0} gap="$control" role="group">
    <Text density="touch" textRole="headline">
      {title}
    </Text>
    {children}
  </Surface>
);

const RequestsList = ({
  onApprove,
  onApproveSelected,
  onDeny,
  onFix,
  onOpen,
  onOpenChannel,
  onRequestChannel,
  onRetry,
  nowMs,
  onTabChange,
  snapshot,
  tab,
}: RequestsListProps) => {
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
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
  if (snapshot.entries.length === 0 && approvals.length === 0)
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
  const failed = requestsInTab(snapshot, "needs-you");
  const bulkable = approvals.length > 1;
  const chosen = approvals.filter((proposal) => selected.has(proposal.id));
  const busy = snapshot.deciding.length > 0;

  const toggle = (id: string, on: boolean) =>
    setSelected((previous) => {
      const next = new Set(previous);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  let content: React.ReactNode;
  if (tab === "needs-you") {
    content =
      approvals.length === 0 && failed.length === 0 ? (
        <StatePanel action={ask} density="touch" kind="empty" {...tabEmpty["needs-you"]} />
      ) : (
        <>
          {approvals.length > 0 ? (
            <Section title="Waiting for your approval">
              {bulkable ? (
                <Surface gap="$inline" level="raised" padding="$control">
                  <Text density="touch" textRole="body" tone="secondary">
                    {chosen.length > 0 ? `${chosen.length} selected` : "None selected"}
                  </Text>
                  <Surface backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap="$inline">
                    <Action
                      accessibilityRole="button"
                      density="touch"
                      onPress={() =>
                        setSelected(
                          chosen.length === approvals.length
                            ? new Set()
                            : new Set(approvals.map((p) => p.id)),
                        )
                      }
                      style={{ flex: 1 }}
                      tone="secondary"
                    >
                      {chosen.length === approvals.length ? "Clear selection" : "Select all"}
                    </Action>
                    <Action
                      accessibilityRole="button"
                      density="touch"
                      disabled={busy || chosen.length === 0}
                      onPress={() => {
                        onApproveSelected(chosen.map((proposal) => proposal.id));
                        setSelected(new Set());
                      }}
                      style={{ flex: 1 }}
                      tone="primary"
                    >
                      {chosen.length > 0 ? `Approve ${chosen.length}` : "Approve selected"}
                    </Action>
                  </Surface>
                </Surface>
              ) : null}
              {approvals.map((proposal) => (
                <ApprovalCard
                  busy={snapshot.deciding.includes(proposal.id)}
                  key={proposal.id}
                  onApprove={() => onApprove(proposal)}
                  onDeny={(reason) => onDeny(proposal, reason)}
                  onToggleSelected={bulkable ? (on) => toggle(proposal.id, on) : undefined}
                  proposal={proposal}
                  selected={selected.has(proposal.id)}
                />
              ))}
            </Section>
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
      {/* Three counted labels can outgrow a narrow phone; the row scrolls rather than clipping one. */}
      <ScrollView horizontal showsHorizontalScrollIndicator={false}>
        <Tabs
          density="touch"
          label="Requests sections"
          onValueChange={onTabChange}
          options={[
            { label: counted("Needs you", requestsNeedsYouCount(snapshot)), value: "needs-you" },
            { label: counted("In progress", inProgress.length), value: "in-progress" },
            { label: counted("Done", done.length), value: "done" },
          ]}
          value={tab}
        />
      </ScrollView>
      {content}
    </Surface>
  );
};

export { RequestsList };
