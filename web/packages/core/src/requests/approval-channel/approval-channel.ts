import type { Proposal } from "@loomarr/api/models/proposal";

// Which channel approving a request will touch, said before anyone confirms (#1817, #1876).
//
// ⚠ Approval binds a channel BY JOB (internal/binder planChannel → channelByIntent): the first
// approval for a job creates a channel, and approving a later revision of the same job replaces
// that channel's lineup. So "Becomes a new channel" is only true when no channel's intentRef is
// the proposal's job id — the summary has to check, not assume.

const CHANNEL_NAME_MAX = 60;

// Mirror of the server's truncateLabel (internal/binder/binder.go): cut at `max`, back up to the
// last space when that keeps more than half, trim trailing punctuation, add an ellipsis. The
// server counts bytes and this counts code points; they agree for every name that fits, and for
// ASCII names that do not.
const truncateLabel = (s: string, max: number): string => {
  const chars = Array.from(s);
  if (chars.length <= max) return s;
  let cut = chars.slice(0, max).join("");
  const space = cut.lastIndexOf(" ");
  if (space > Math.floor(max / 2)) cut = cut.slice(0, space);
  return `${cut.replace(/[ ,.;:-]+$/, "")}…`;
};

// The name a NEW channel gets on approval — the server's channelNameFromIntent, so the preview
// never names a channel the server will not create.
const newChannelName = (proposal: Pick<Proposal, "channelName" | "intent">): string => {
  const named = proposal.channelName?.trim();
  if (named) return truncateLabel(named, CHANNEL_NAME_MAX);
  const described = proposal.intent?.description?.trim();
  if (described) return truncateLabel(described, CHANNEL_NAME_MAX);
  return "Suggested channel";
};

type ApprovalChannelEffect = { kind: "create"; name: string } | { kind: "update"; name: string };

// What approving does to channels: update the one already bound to this job, or create one.
const approvalChannelEffect = (
  jobId: string,
  proposal: Pick<Proposal, "channelName" | "intent">,
  channels: readonly { intentRef?: string; name: string }[],
): ApprovalChannelEffect => {
  const bound = jobId ? channels.find((ch) => ch.intentRef === jobId) : undefined;
  return bound ? { kind: "update", name: bound.name } : { kind: "create", name: newChannelName(proposal) };
};

export type { ApprovalChannelEffect };
export { approvalChannelEffect, newChannelName };
