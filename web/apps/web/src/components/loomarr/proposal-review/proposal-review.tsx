import type { ProposalItem } from "@loomarr/api";
import { formatPercent } from "@loomarr/core";
import { Pencil } from "lucide-react";
import type { ReactNode } from "react";
import { Badge, Button, Card } from "@/components/ui";
import { cn } from "@/lib";
import type { ProposalReviewProps, ProposalStatus } from "./proposal-review.type";

// ProposalReview — the human-in-the-loop review that fronts the approval gate (§3,
// §8). It shows the grounded lineup (in-library, ready now), the acquisitions it
// wants (missing titles → the provisioner, but only on approve), ranked alternates,
// and the model's rationale + deterministic scores. Nothing here executes; approve/
// deny IS the gate (§7). Each row is editable-via-search (onEditItem) so a pick can
// be swapped for a grounded alternative without regenerating the whole proposal.
const STATUS: Record<
  ProposalStatus,
  { label: string; variant: "neutral" | "suggest" | "lock" | "onair" | "caution" }
> = {
  draft: { label: "Draft", variant: "neutral" },
  submitted: { label: "Awaiting approval", variant: "suggest" },
  approved: { label: "Approved", variant: "lock" },
  denied: { label: "Denied", variant: "onair" },
  "partially-edited": { label: "Edited", variant: "caution" },
};

const ItemRow = ({
  item,
  kind,
  onEdit,
}: {
  item: ProposalItem;
  kind: "lineup" | "acquire";
  onEdit?: (item: ProposalItem) => void;
}) => (
  <li className="flex items-start gap-3 rounded-md border border-border bg-card px-3 py-2.5">
    <div className="min-w-0 flex-1">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-sm">{item.name}</span>
        {item.year ? <span className="font-mono text-static-400 text-xs">{item.year}</span> : null}
        <Badge variant={kind === "lineup" ? "lock" : "tune"}>
          {kind === "lineup" ? "In library" : "Will acquire"}
        </Badge>
        {typeof item.confidence === "number" && (
          <span className="font-mono text-static-400 text-xs">{`${formatPercent(item.confidence)} fit`}</span>
        )}
      </div>
      {item.rationale && <p className="mt-1 text-muted-foreground text-sm">{item.rationale}</p>}
    </div>
    {onEdit && (
      <Button
        variant="ghost"
        size="icon"
        className="size-7 shrink-0"
        aria-label={`Swap ${item.name}`}
        onClick={() => onEdit(item)}
      >
        <Pencil aria-hidden />
      </Button>
    )}
  </li>
);

const Section = ({ title, count, children }: { title: string; count: number; children: ReactNode }) =>
  count === 0 ? null : (
    <section className="flex flex-col gap-2">
      <h3 className="font-mono text-static-400 text-xs uppercase tracking-wide">{`${title} · ${count}`}</h3>
      <ul className="flex flex-col gap-1.5">{children}</ul>
    </section>
  );

const ProposalReview = ({
  proposal,
  status = "draft",
  busy = false,
  onApprove,
  onDeny,
  onEditItem,
  className,
}: ProposalReviewProps) => {
  const s = STATUS[status];
  const actionable = status === "draft" || status === "submitted" || status === "partially-edited";
  // The generated arrays are `ProposalItem[] | null` (a nil Go slice marshals as null).
  const lineup = proposal.lineup ?? [];
  const acquisitions = proposal.acquisitions ?? [];
  const alternates = proposal.alternates ?? [];
  return (
    <Card className={cn("flex flex-col gap-5 p-5", className)}>
      <header className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="font-semibold text-lg">Proposed lineup</h2>
            <Badge variant={s.variant}>{s.label}</Badge>
          </div>
          {proposal.rationale && (
            <p className="mt-1 max-w-prose text-muted-foreground text-sm">{proposal.rationale}</p>
          )}
        </div>
        {proposal.scores && (
          <dl className="shrink-0 text-right">
            <div className="flex items-baseline justify-end gap-1.5">
              <dt className="text-static-400 text-xs">Theme fit</dt>
              <dd className="font-mono text-sm text-suggest-300">
                {formatPercent(proposal.scores.themeFit)}
              </dd>
            </div>
            <div className="flex items-baseline justify-end gap-1.5">
              <dt className="text-static-400 text-xs">Ready now</dt>
              <dd className="font-mono text-lock text-sm">
                {formatPercent(proposal.scores.availabilityRatio)}
              </dd>
            </div>
          </dl>
        )}
      </header>

      <Section title="Lineup" count={lineup.length}>
        {lineup.map((item) => (
          <ItemRow key={item.name} item={item} kind="lineup" onEdit={onEditItem} />
        ))}
      </Section>

      <Section title="Acquisitions" count={acquisitions.length}>
        {acquisitions.map((item) => (
          <ItemRow key={item.name} item={item} kind="acquire" onEdit={onEditItem} />
        ))}
      </Section>

      {alternates.length > 0 && (
        <section className="flex flex-col gap-1.5">
          <h3 className="font-mono text-static-400 text-xs uppercase tracking-wide">
            {`Alternates · ${alternates.length}`}
          </h3>
          <p className="text-muted-foreground text-sm">{alternates.map((a) => a.name).join(" · ")}</p>
        </section>
      )}

      {actionable && (
        <footer className="flex justify-end gap-2 border-border border-t pt-4">
          <Button variant="outline" onClick={onDeny} disabled={busy}>
            Deny
          </Button>
          <Button onClick={onApprove} disabled={busy}>
            Approve & acquire
          </Button>
        </footer>
      )}
    </Card>
  );
};

export { ProposalReview };
