import type { PoolChannelDTO } from "@loomarr/api";
import { pluralize } from "@loomarr/core";
import { Button, Caption } from "@/components/ui";
import { cn } from "@/lib";
import type { PoolHealthProps } from "./pool-health.type";

// PoolHealth — catalog-wide filler health, above every tab on the Filler page (V35).
//
// ⚠ **A strip, not a tab, and that is the design rather than a layout preference.** Catalog
// health is the context the other three tabs are read in: what the catalog holds decides whether
// the clips you are tagging matter and whether a pull is worth proposing. As its own tab it was
// the thing nobody clicked, which is how an install ends up with four hundred clips and channels
// still falling back to the bumper card.
//
// ⚠ **Every per-channel number here is the SAME answer the channel page's coverage meter gives** —
// the server computes it by calling `Coverage` once per live channel, so there is no aggregate
// ladder to drift. That agreement is pinned by a Go test, not by this component agreeing to be
// careful.

// How a channel's ladder rung reads to an operator. Keyed by the generated union so a new rung
// is a compile error here rather than a blank cell.
const LEVEL_LABEL: Record<PoolChannelDTO["level"], string> = {
  exact: "exact match",
  widened: "same decade",
  audience: "audience only",
  bumper_card: "no commercials",
};

// One number and its explanation. `note` is what stops a bare figure being a riddle: 61 means
// nothing until you know it is 61 of 90.
const Stat = ({
  label,
  value,
  note,
  tone,
}: {
  label: string;
  value: string;
  note: string;
  tone?: "warn";
}) => (
  <div className="flex min-w-0 flex-col gap-0.5">
    <Caption className="uppercase tracking-wide">{label}</Caption>
    <span className={cn("font-semibold text-lg tabular-nums", tone === "warn" && "text-lock")}>{value}</span>
    <span className="text-muted-foreground text-xs">{note}</span>
  </div>
);

const PoolHealth = ({ pool, onProposePull, proposing, className }: PoolHealthProps) => {
  // ⚠ `?? []` because huma types every Go slice as nullable, so the generated DTO says
  // `PoolChannelDTO[] | null` even though the handler always sends `[]`. Same unwrap the
  // coverage meter and channel-filler use.
  const channels = pool.channels ?? [];
  // The server sorts worst-first, so the head is the channel to fix. Reading it positionally
  // rather than re-sorting keeps one definition of "worst" — the ladder's.
  const weakest = channels[0];
  const covered = channels.filter((c) => c.level === "exact").length;
  const onCardOnly = channels.filter((c) => c.level === "bumper_card").length;
  // ⚠ The MIDDLE of the ladder — channels that do play commercials, just not well-matched ones.
  // Derived as "neither end" rather than by listing `widened`/`audience`, so a new rung lands here
  // (accurately: it is a looser match) instead of silently vanishing from the note.
  const looseMatch = channels.length - covered - onCardOnly;

  return (
    <section
      aria-label="Catalog health"
      className={cn(
        "flex flex-wrap items-center gap-x-8 gap-y-4 rounded-lg border border-border p-4",
        className,
      )}
    >
      <Stat
        label="Clips"
        value={String(pool.clips)}
        note={
          pool.untagged > 0
            ? `${pluralize(pool.untagged, "clip")} still ${pool.untagged === 1 ? "needs" : "need"} tagging`
            : "all tagged"
        }
      />

      {/* ⚠ The number that surprises people, which is why it is a headline and not a footnote:
          a catalog of five hundred fifteen-minute compilations reads as healthy by clip count
          and can fill nothing. It is the same duration gate pod assembly applies. */}
      <Stat
        label="Fits a break"
        value={String(pool.eligible)}
        note={`of ${pluralize(pool.commercials, "commercial")}`}
        {...(pool.commercials > 0 && pool.eligible === 0 ? { tone: "warn" as const } : {})}
      />

      {/* ⚠ **The note has to describe the same population the fraction counts.** It said "every
          channel has commercials" whenever nothing had fallen to the bumper card — true, but about
          a DIFFERENT question than the headline, which counts channels matching EXACTLY. An
          install where all four sat on the middle rung read "2/4 · every channel has commercials",
          two numbers that look like a contradiction and are both correct.

          The middle rung is the interesting state and had no words at all: those channels do play
          commercials, just poorly matched ones. Naming it is what turns the fraction into
          something an operator can act on. Seen live 2026-08-03. */}
      {channels.length > 0 && (
        <Stat
          label="Channels covered"
          value={`${covered}/${channels.length}`}
          note={
            onCardOnly > 0
              ? `${pluralize(onCardOnly, "channel")} ${onCardOnly === 1 ? "has" : "have"} nothing to play`
              : looseMatch > 0
                ? `${pluralize(looseMatch, "channel")} ${looseMatch === 1 ? "falls" : "fall"} back to a looser match`
                : "every channel matches exactly"
          }
          {...(onCardOnly > 0 ? { tone: "warn" as const } : {})}
        />
      )}

      {/* The diagnosis. Naming the channel is the difference between "something is thin" and
          something an operator can act on, and it costs nothing because the server already
          ordered the list. Omitted when every channel matches exactly: a callout that is always
          on teaches people to stop reading it. */}
      {weakest && weakest.level !== "exact" && (
        <Stat
          label="Weakest"
          value={weakest.name}
          note={`breaks fall back to ${LEVEL_LABEL[weakest.level] ?? weakest.level}`}
          tone="warn"
        />
      )}

      {onProposePull && (
        <Button className="ml-auto" onClick={onProposePull} disabled={proposing}>
          {proposing ? "Planning…" : "Propose a pull"}
        </Button>
      )}
    </section>
  );
};

export { PoolHealth };
