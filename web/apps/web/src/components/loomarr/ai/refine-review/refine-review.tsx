import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { DateScope } from "@loomarr/api/models/dateScope";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { Range } from "@loomarr/api/models/range";
import { tidyGoDuration } from "@loomarr/core/format";
import { Action, Card, Icon, icons, Stack, Text } from "@loomarr/design-system";
import { seasonalModeLabel } from "@loomarr/ui";
import { seasonalModeOf } from "@/components/loomarr/channels/channel-seasonal";
import { eraOf } from "@/lib/era-dates";
import { playOrderLabel } from "@/lib/play-order-copy";
import { datesMode } from "@/lib/policy-overrides";
import { cn } from "@/lib/utils";
import { friendlyTitleRationale } from "@/suggest/suggestion-language";
import type { CurrentLineupItem, RefineReviewProps } from "./refine-review.type";

// --- programming-policy deltas (§8.2) --------------------------------------------------
// A refine can change what the LLM owns (scope/audience/ordering/seasonal). Shown here so
// none applies invisibly; this is the visible half of operator-dirty stickiness.
// A field the operator PINNED is shown as "kept" — the refine can't overwrite it.
interface PolicyDelta {
  label: string;
  from: string;
  to: string;
  pinned: boolean; // operator set this field → the refine leaves it alone (P3 stickiness)
}

const rangeLabel = (range?: Range): string =>
  !range || (!range.from && !range.to) ? "Any" : `${range.from ?? "…"}–${range.to ?? "…"}`;

const DATE_AXES = [
  ["Movie release", "movieRelease"],
  ["Series premiere", "seriesPremiere"],
  ["Episode airing", "seriesAiring"],
] as const;

// The one stored date scope (#1883), read the way the editor shows it: an era when the dates are
// era-shaped or empty, otherwise each constrained axis and its ranges.
const datesLabel = (dates?: DateScope): string =>
  datesMode(dates) === "era"
    ? rangeLabel(eraOf(dates))
    : DATE_AXES.filter(([, axis]) => dates?.[axis]?.length)
        .map(([label, axis]) => `${label} ${(dates?.[axis] ?? []).map(rangeLabel).join(", ")}`)
        .join(" · ");

// A no-repeat window, tidied so an unchanged "168h" and "168h0m0s" do not read as a diff. A zero
// or absent window means no restriction — say so rather than showing an empty cell.
const noRepeatLabel = (v?: string): string => tidyGoDuration(v) || "No restriction";

const policyDeltas = (current?: ChannelPolicy, proposed?: ChannelPolicy): PolicyDelta[] => {
  if (!current || !proposed) return [];
  const pinned = new Set(current.operatorSet ?? []);
  const out: PolicyDelta[] = [];
  const add = (label: string, path: string, from: string, to: string) => {
    if (from !== to) out.push({ label, from, to, pinned: pinned.has(path) });
  };
  // Labelled like the editor's field: Era while both sides are era-shaped, otherwise dates.
  const eraRow = datesMode(current.scope?.dates) === "era" && datesMode(proposed.scope?.dates) === "era";
  add(
    eraRow ? "Era" : "Programming dates",
    "scope",
    datesLabel(current.scope?.dates),
    datesLabel(proposed.scope?.dates),
  );
  add(
    "Audience ceiling",
    "audience",
    current.audience?.ceiling || "No limit",
    proposed.audience?.ceiling || "No limit",
  );
  // Named as the editor names them, so the review and the setting it changes read the same.
  add("Ordering", "ordering", playOrderLabel(current.ordering), playOrderLabel(proposed.ordering));
  add(
    "Seasonal",
    "seasonal",
    seasonalModeLabel(seasonalModeOf(current.seasonal?.mode)),
    seasonalModeLabel(seasonalModeOf(proposed.seasonal?.mode)),
  );
  // Separation is refreshed by MergeFromProposal exactly like the four above, so leaving it
  // out meant a refine could silently widen or drop a no-repeat window with nothing shown
  // before Approve. Two rows, not one: the pinned check is per-path, and "which window
  // changed" is the useful part.
  add(
    "No-repeat · movies",
    "separation",
    noRepeatLabel(current.separation?.movieNoRepeat),
    noRepeatLabel(proposed.separation?.movieNoRepeat),
  );
  add(
    "No-repeat · episodes",
    "separation",
    noRepeatLabel(current.separation?.episodeNoRepeat),
    noRepeatLabel(proposed.separation?.episodeNoRepeat),
  );
  return out;
};

const PolicyDeltaGroup = ({ deltas }: { deltas: PolicyDelta[] }) => {
  if (deltas.length === 0) return null;
  return (
    <Stack gap="1.5" render="section">
      <h3 className="text-2xs text-static-400">{`Programming · ${deltas.length}`}</h3>
      <Stack gap="1.5" render="ul">
        {deltas.map((d) => (
          <li key={d.label} className="flex flex-wrap items-baseline gap-x-2">
            <Text as="span" textRole="compact" tone="secondary">
              {d.label}
            </Text>
            {d.pinned ? (
              <Text as="span" textRole="compact" tone="secondary">
                keeping <span className="text-foreground">{d.from}</span>, you set this; the AI suggested{" "}
                {d.to}
              </Text>
            ) : (
              <Text as="span" textRole="compact">
                <span className="text-static-400 line-through">{d.from}</span>
                <span className="text-muted-foreground mx-1" aria-hidden>
                  →
                </span>
                <span className="text-foreground">{d.to}</span>
              </Text>
            )}
          </li>
        ))}
      </Stack>
    </Stack>
  );
};

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
  // The LLM's why-it-fits for this title. Carried through the diff because it was
  // already in the payload and thrown away one function before render — the refine
  // was asking an operator to accept picks it had explanations for but didn't show.
  // Only meaningful on ADDED rows: a removal needs no justification from the model,
  // and a kept title was already the operator's choice.
  rationale?: string;
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
    const row = { key, name: item.name, year: item.year, rationale: item.rationale };
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
  <Stack gap="0.5" render="li">
    <Stack align="baseline" direction="row" gap="inline">
      <Stack align="center" aria-hidden className="size-4" flexShrink={0} justify="center" render="span">
        {tone === "kept" && <Icon decorative glyph={icons.selected} size="compact" tone="muted" />}
        {tone === "added" && <Icon decorative glyph={icons.selected} size="compact" tone="success" />}
        {tone === "removed" && <Icon decorative glyph={icons.remove} size="compact" tone="danger" />}
      </Stack>
      <Text
        as="span"
        className={cn(tone === "removed" && "line-through")}
        textRole="compact"
        tone={tone === "kept" ? "secondary" : tone === "removed" ? "danger" : "primary"}
      >
        {row.name}
      </Text>
      {row.year ? (
        <Text as="span" textRole="mono" tone="secondary">
          {row.year}
        </Text>
      ) : null}
      {note && (
        <Text as="span" textRole="hint" tone="secondary">
          {note}
        </Text>
      )}
    </Stack>
    {/* Only on additions. A removal doesn't need the model's justification, and a kept
        title was the operator's pick in the first place — but "Adding: Predator" with no
        reason is the refine asking for trust it hasn't earned. */}
    {tone === "added" && row.rationale && (
      <Text as="p" className="pl-6" textRole="hint" tone="secondary">
        {friendlyTitleRationale(row.rationale)}
      </Text>
    )}
  </Stack>
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
    <Stack gap="1.5" render="section">
      <h3 className="text-2xs text-static-400">{`${title} · ${rows.length}`}</h3>
      <Stack gap="1" render="ul">
        {rows.map((row) => (
          <Row key={row.key} row={row} tone={tone} note={rowNote} />
        ))}
      </Stack>
      {emptyNote}
    </Stack>
  );
};

const RefineReview = ({
  proposed,
  acquisitions = [],
  current = [],
  currentPolicy,
  proposedPolicy,
  onApply,
  onDiscard,
  busy = false,
}: RefineReviewProps) => {
  const { kept, added, removed } = diffLineup(proposed, current);
  const deltas = policyDeltas(currentPolicy, proposedPolicy);
  // Acquisitions are new-by-definition (not yet in the library, let alone on the
  // channel), so they always read as "adding" — the diff itself never has to place
  // them, but the copy calls out that they need downloading, unlike a plain add.
  const acquiring: DiffRow[] = acquisitions.map((item) => ({
    key: keyOf(item),
    name: item.name,
    year: item.year,
    rationale: item.rationale,
  }));

  const noChanges =
    added.length === 0 && removed.length === 0 && acquiring.length === 0 && deltas.length === 0;

  return (
    <Card gap={16} padding={20}>
      <header>
        <Text as="h2" strong textRole="headline">
          Refined lineup
        </Text>
        <Text as="p" textRole="compact" tone="secondary">
          {noChanges
            ? "No changes. Your channel already matches."
            : "Review what changes before applying it."}
        </Text>
      </header>

      {noChanges ? (
        <Text as="p" textRole="compact">
          The suggestions are already on this channel.
        </Text>
      ) : (
        <Stack gap="4">
          <Group title="Keeping" rows={kept} tone="kept" />
          <Group title="Adding" rows={added} tone="added" />
          {acquiring.length > 0 && <Group title="Adding (needs downloading)" rows={acquiring} tone="added" />}
          <Group title="Removing" rows={removed} tone="removed" />
          <PolicyDeltaGroup deltas={deltas} />
        </Stack>
      )}

      <Stack
        className="border-border border-t pt-4"
        direction="row"
        gap="inline"
        justify="end"
        render="footer"
      >
        <Action disabled={busy} onPress={onDiscard} variant="ghost">
          Discard
        </Action>
        {!noChanges && (
          <Action disabled={busy} icon="download" onPress={onApply} tone="suggest">
            Apply changes
          </Action>
        )}
      </Stack>
    </Card>
  );
};

export type { DiffRow };
export { RefineReview };
