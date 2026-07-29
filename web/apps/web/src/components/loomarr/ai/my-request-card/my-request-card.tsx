import { Badge, Card } from "@/components/ui";
import { cn } from "@/lib";
import type { MyRequestCardProps } from "./my-request-card.type";

// MyRequestCard — one submitted request, from the requester's side (V26 / `A2`, §12).
//
// The defect this closes: a member could submit a proposal and then see NOTHING. The queue
// page fanned `GET /v1/titles` across provisioning states, which is the *titles* half of a
// request; the proposal itself — submitted, approved, denied, or approved-with-changes — had
// no data source in the web app at all. Approval provenance was worse than absent: `approvedBy`
// was stored and rendered nowhere, and V25b's note reached the database with nothing to display
// it.
//
// So this card's job is to answer, in the requester's words, "what happened to what I asked
// for?" — and especially the two cases where silence is worst: it came back CHANGED, or it was
// declined.

const STATUS: Record<string, { label: string; variant: "suggest" | "lock" | "onair" | "caution" }> = {
  submitted: { label: "Waiting for approval", variant: "suggest" },
  approved: { label: "Approved", variant: "lock" },
  denied: { label: "Not approved", variant: "onair" },
};

const MyRequestCard = ({ proposal, className }: MyRequestCardProps) => {
  const status = STATUS[proposal.status] ?? { label: proposal.status, variant: "caution" as const };
  // An edited approval is a distinct outcome from a plain one — "approved" alone would hide
  // that the lineup someone gets is not the lineup they asked for.
  const changed = proposal.status === "approved" && Boolean(proposal.modSummary);

  const title = proposal.proposal?.intent?.description ?? "Suggested lineup";
  const lineupCount = proposal.proposal?.lineup?.length ?? 0;
  const acquireCount = proposal.proposal?.acquisitions?.length ?? 0;

  return (
    <Card className={cn("flex flex-col gap-2 p-4", className)}>
      <div className="flex flex-wrap items-center gap-2">
        <p className="min-w-0 flex-1 font-medium">{title}</p>
        <Badge variant={changed ? "caution" : status.variant}>
          {changed ? "Approved with changes" : status.label}
        </Badge>
      </div>

      <p className="text-muted-foreground text-sm">
        {`${lineupCount + acquireCount} titles`}
        {acquireCount > 0 ? ` · ${acquireCount} to acquire` : ""}
      </p>

      {/* CHANGED BY — the gate's phrase. `modSummary` is generated server-side ("dropped 2,
          added 1"), so it is a record of what happened rather than a claim someone typed;
          `approvedBy` names who. Shown together because either alone is unsatisfying: a change
          with no author, or an author with no change. */}
      {changed && (
        <p className="text-caution text-sm">
          <span className="font-mono text-2xs uppercase tracking-wide">
            {proposal.approvedBy ? `Changed by ${proposal.approvedBy}` : "Changed"}
          </span>
          {`, ${proposal.modSummary}`}
        </p>
      )}

      {/* The approver's own words, on any outcome that carries them. This is the half of the
          feature that is about people: a request that comes back altered without explanation
          reads as arbitrary. */}
      {proposal.note && <p className="text-sm">{proposal.note}</p>}

      {/* The denial line. A member who is told only "not approved" has learned nothing and will
          submit the same intent again — which is why V23 captured the reason and why it has to
          surface HERE, on the requester's side, not only in the admin queue. */}
      {proposal.status === "denied" && (
        <p className="text-onair-300 text-sm">
          {proposal.denyReason ? proposal.denyReason : "No reason was given."}
        </p>
      )}
    </Card>
  );
};

export { MyRequestCard };
