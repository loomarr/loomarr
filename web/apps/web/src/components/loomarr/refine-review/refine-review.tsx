import type { ProposalItem } from "@loomarr/api";
import { Check, Download, Minus } from "lucide-react";
import { Button, Card } from "@/components/ui";
import { cn } from "@/lib";
import type { CurrentLineupItem, RefineReviewProps } from "./refine-review.type";

// RefineReview — the diff a "Refine with AI" run lands on (§7 refine). The refined
// proposal is the new desired lineup; `current` (ChannelDTO.lineup) is what's on the
// channel today. Diffing them by identity — never by array position — is what makes
// "Removing: Point Break / Adding: Predator" trustworthy rather than a guess.
//
// Identity is the provisioning key (§3: "movie:tmdb:603", "series:tvdb:71663"), the same
// key LineupEntryDTO carries and ProposalItem.Key() derives server-side from
// mediaType+tmdbId+tvdbId — deriving it client-side the identical way means a kept item
// matches exactly, not approximately. Only an item with no usable id (which the
// grounding gate is not supposed to let through) falls back to name+year.
const keyOf = (item: ProposalItem): string => {
  if (item.mediaType === "series" && item.tvdbId) return `series:tvdb:${item.tvdbId}`;
  if (item.tmdbId) return `${item.mediaType}:tmdb:${item.tmdbId}`;
  return `name:${item.name.trim().toLowerCase()}:${item.year ?? ""}`;
};

const currentKeyOf = (item: CurrentLineupItem): string =>
  item.key ?? `name:${item.name.trim().toLowerCase()}:${item.year ?? ""}`;

interface DiffRow {
  key: string;
  name: string;
  year?: number;
}

interface Diff {
  kept: DiffRow[];
  added: DiffRow[];
  removed: DiffRow[];
}

const diffLineup = (proposed: ProposalItem[], current: CurrentLineupItem[]): Diff => {
  const proposedByKey = new Map(proposed.map((item) => [keyOf(item), item]));
  const currentByKey = new Map(current.map((item) => [currentKeyOf(item), item]));

  const kept: DiffRow[] = [];
  const added: DiffRow[] = [];
  for (const [key, item] of proposedByKey) {
    const row = { key, name: item.name, year: item.year };
    if (currentByKey.has(key)) kept.push(row);
    else added.push(row);
  }

  const removed: DiffRow[] = [];
  for (const [key, item] of currentByKey) {
    if (!proposedByKey.has(key)) removed.push({ key, name: item.name, year: item.year });
  }

  return { kept, added, removed };
};

const Row = ({ row, tone, note }: { row: DiffRow; tone: "kept" | "added" | "removed"; note?: string }) => (
  <li className="flex items-baseline gap-2 text-sm">
    <span className="flex size-4 shrink-0 items-center justify-center" aria-hidden>
      {tone === "kept" && <Check className="size-3.5 text-static-400" />}
      {tone === "added" && <Check className="size-3.5 text-lock" />}
      {tone === "removed" && <Minus className="size-3.5 text-onair-300" />}
    </span>
    <span
      className={cn(
        tone === "kept" && "text-static-400",
        tone === "added" && "text-foreground",
        tone === "removed" && "text-onair-300 line-through",
      )}
    >
      {row.name}
    </span>
    {row.year ? <span className="font-mono text-static-400 text-xs">{row.year}</span> : null}
    {note && <span className="text-static-400 text-xs">{note}</span>}
  </li>
);

const Group = ({
  title,
  rows,
  tone,
  emptyNote,
  rowNote,
}: {
  title: string;
  rows: DiffRow[];
  tone: "kept" | "added" | "removed";
  emptyNote?: string;
  rowNote?: string;
}) => {
  if (rows.length === 0) return null;
  return (
    <section className="flex flex-col gap-1.5">
      <h3 className="font-mono text-static-400 text-xs uppercase tracking-wide">{`${title} · ${rows.length}`}</h3>
      <ul className="flex flex-col gap-1">
        {rows.map((row) => (
          <Row key={row.key} row={row} tone={tone} note={rowNote} />
        ))}
      </ul>
      {emptyNote}
    </section>
  );
};

const RefineReview = ({
  proposed,
  acquisitions = [],
  current = [],
  onApply,
  onDiscard,
  busy = false,
  className,
}: RefineReviewProps) => {
  const { kept, added, removed } = diffLineup(proposed, current);
  // Acquisitions are new-by-definition (not yet in the library, let alone on the
  // channel), so they always read as "adding" — the diff itself never has to place
  // them, but the copy calls out that they need downloading, unlike a plain add.
  const acquiring: DiffRow[] = acquisitions.map((item) => ({
    key: keyOf(item),
    name: item.name,
    year: item.year,
  }));

  const noChanges = added.length === 0 && removed.length === 0 && acquiring.length === 0;

  return (
    <Card className={cn("flex flex-col gap-4 p-5", className)}>
      <header>
        <h2 className="font-semibold text-lg">Refined lineup</h2>
        <p className="text-muted-foreground text-sm">
          {noChanges
            ? "No changes — your channel already matches."
            : "Review what changes before applying it."}
        </p>
      </header>

      {noChanges ? (
        <p className="text-sm">Everything the model proposed is already on this channel.</p>
      ) : (
        <div className="flex flex-col gap-4">
          <Group title="Keeping" rows={kept} tone="kept" />
          <Group title="Adding" rows={added} tone="added" />
          {acquiring.length > 0 && <Group title="Adding (needs downloading)" rows={acquiring} tone="added" />}
          <Group title="Removing" rows={removed} tone="removed" />
        </div>
      )}

      <footer className="flex justify-end gap-2 border-border border-t pt-4">
        <Button variant="ghost" onClick={onDiscard} disabled={busy}>
          Discard
        </Button>
        {!noChanges && (
          <Button variant="suggest" onClick={onApply} disabled={busy}>
            <Download aria-hidden />
            Apply changes
          </Button>
        )}
      </footer>
    </Card>
  );
};

export type { DiffRow };
export { RefineReview };
