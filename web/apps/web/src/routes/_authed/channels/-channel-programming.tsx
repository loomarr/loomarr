import type { ChannelPolicy, LineupEntryDTO } from "@loomarr/api";
import {
  ChannelCyclePreview,
  ChannelLineupEditor,
  ChannelPolicyFields,
  ChannelRulesEditor,
  RefinePanel,
} from "@/components/loomarr";

// ChannelProgramming — the unified "what plays, and when" surface (design.md §12). It folds
// what used to be three peer tabs (Lineup, Programming rules, Refine with AI) into ONE surface
// with a visible hierarchy, so an operator stops guessing which tab shapes what:
//
//   Refine with AI (the header affordance — describe a change, review a diff, apply)
//   1. What plays      — the deck (lineup) + the content it's limited to (era + ceiling)
//   2. How it's ordered — the order + spacing programs play in
//   3. When it changes  — wall-clock curation rules (marathons, holidays, dayparts)
//   · Preview          — one shared "what airs at a moment" pane, docked at the bottom
//
// Every edit saves seamlessly and auto-reconciles (§9), except the two review-before-apply
// affordances that say so: Refine (a diff) and the Filler sandbox (its own tab). The single
// cycle preview replaces the old split between the series-level lineup and a separate
// episode-level "preview what airs" list.

interface ChannelProgrammingProps {
  channelId: string;
  channelName: string;
  lineup: LineupEntryDTO[];
  policy: ChannelPolicy;
  onPolicyChange: (next: ChannelPolicy) => void;
  onRefined: () => void;
}

// Block — one titled step of the Programming surface. The numberless heading + one-line hint
// name what the block controls in the operator's words (not the schema's).
const Block = ({ title, hint, children }: { title: string; hint: string; children: React.ReactNode }) => (
  <section className="flex flex-col gap-4 border-border border-t pt-6">
    <div>
      <h3 className="font-semibold text-base">{title}</h3>
      <p className="text-muted-foreground text-sm">{hint}</p>
    </div>
    {children}
  </section>
);

const ChannelProgramming = ({
  channelId,
  channelName,
  lineup,
  policy,
  onPolicyChange,
  onRefined,
}: ChannelProgrammingProps) => {
  const lineupKeys = lineup.map((e) => ({ key: e.key, title: e.name }));

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="font-semibold text-lg">Programming</h2>
        <p className="text-muted-foreground text-sm">What this channel plays, and when.</p>
      </div>

      {/* Refine with AI — the header affordance, not a peer tab. Describe a change, review the
          diff, apply. It acts on the SAME lineup + policy the blocks below edit by hand. */}
      <RefinePanel
        channelId={channelId}
        channelName={channelName}
        current={lineup.map((entry) => ({ name: entry.name, year: entry.year, key: entry.key }))}
        currentPolicy={policy}
        onApplied={onRefined}
      />

      <Block title="What plays" hint="The titles this channel draws from, and the content it stays within.">
        <ChannelLineupEditor channelId={channelId} lineup={lineup} />
        <ChannelPolicyFields policy={policy} onChange={onPolicyChange} show="scope" />
      </Block>

      <Block title="How it's ordered" hint="The order and spacing programs play in.">
        <ChannelPolicyFields policy={policy} onChange={onPolicyChange} show="ordering" />
      </Block>

      <Block
        title="When it changes"
        hint="Play different things at different times — weekend marathons, holiday blocks, day-parts."
      >
        <ChannelRulesEditor policy={policy} onChange={onPolicyChange} lineupKeys={lineupKeys} />
      </Block>

      {/* One shared preview: time-travel the schedule to see exactly what airs — and which rule
          wins — at any moment. Verifies the deck, the ordering, AND the rules above. */}
      <div className="border-border border-t pt-6">
        <ChannelCyclePreview channelId={channelId} />
      </div>
    </div>
  );
};

export { ChannelProgramming };
