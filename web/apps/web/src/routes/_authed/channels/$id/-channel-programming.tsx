import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { LineupEntryDTO } from "@loomarr/api/models/lineupEntryDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { useChannelRulesDraft } from "@/channels/use-channel-rules-draft";
import { RefinePanel } from "@/components/loomarr/ai/refine-panel";
import { ChannelAutoCurate } from "@/components/loomarr/channels/channel-auto-curate";
import { ChannelCollectionsScope } from "@/components/loomarr/channels/channel-collections-scope";
import { ChannelCyclePreview } from "@/components/loomarr/channels/channel-cycle-preview";
import { ChannelLineupEditor } from "@/components/loomarr/channels/channel-lineup-editor";
import { ChannelPolicyFields } from "@/components/loomarr/channels/channel-policy-fields";
import { ChannelRulesEditor } from "@/components/loomarr/channels/channel-rules-editor";
import { ChannelSeasonal } from "@/components/loomarr/channels/channel-seasonal";
import { ChannelSeriesScope } from "@/components/loomarr/channels/channel-series-scope";
import { CollapsibleSection } from "@/components/loomarr/feedback/collapsible-section";
import { Button } from "@/components/ui/button";

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
// Most edits save seamlessly and auto-reconcile (§9). The review-before-apply affordances say
// so: Refine (a diff), the Filler sandbox (its own tab), and the scheduling RULES below.
//
// ⚠ Rules are the odd one out on this page, and deliberately (§12): they resolve
// first-match-by-priority, so a rule's effect depends on every rule above it and each
// intermediate state is a different schedule. Everything else here — lineup, era/ceiling,
// ordering, seasonal, auto-curate — is self-contained and stays seamless.
//
// The single cycle preview replaces the old split between the series-level lineup and a
// separate episode-level "preview what airs" list. It follows the rules DRAFT while one is
// dirty, so what you are looking at is what applying would ship.

interface ChannelProgrammingProps {
  channelId: string;
  revision: number;
  channelName: string;
  lineup: LineupEntryDTO[];
  policy: ChannelPolicy;
  onPolicyChange: (next: ChannelPolicy) => void;
  // The channel's stored playback strategy. Not part of ChannelPolicy and read only here:
  // policy.ordering is the one editable play-order knob, whose unset option names this fallback.
  strategy?: string;
  // The channel's stored intent (`ChannelDTO.intentRef`). Auto-curate re-runs that intent
  // (programming-design.md §8.2), so a hand-made channel has nothing to re-evaluate — the
  // control says so instead of offering a setting the job would skip.
  intentRef?: string;
  onRefined: () => void;
}

// Block — one intent-sized step of the Programming surface. The first task opens; secondary
// tuning stays quiet until requested. Closed content remains reachable to find-in-page through
// the shared disclosure primitive.
const Block = ({
  title,
  hint,
  defaultOpen,
  children,
}: {
  title: string;
  hint: string;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) => (
  <CollapsibleSection title={title} description={hint} defaultOpen={defaultOpen}>
    <div className="flex flex-col gap-4">{children}</div>
  </CollapsibleSection>
);

const ChannelProgramming = ({
  channelId,
  revision,
  channelName,
  lineup,
  policy,
  onPolicyChange,
  strategy,
  intentRef,
  onRefined,
}: ChannelProgrammingProps) => {
  const lineupKeys = lineup.map((e) => ({ key: e.key, title: e.name }));

  // The scheduling-rules draft (§12). Only the rules block reads it; everything else on this
  // surface keeps saving inline through `onPolicyChange`.
  const rules = useChannelRulesDraft(channelId, policy, revision);

  // The rule authoring vocabulary (§6.6) is served by the BE so the rules editor no longer
  // hand-mirrors the lowering table. Static per build → cache forever; the editor renders once
  // it lands (a gate below), so its drafts are never derived against an empty vocabulary.
  const vocabQuery = channelsApi.useGetProgrammingVocabulary({
    query: { staleTime: Number.POSITIVE_INFINITY },
  });
  const vocabulary = unwrap(vocabQuery.data);

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

      <Block
        title="What plays"
        hint="The titles this channel draws from, and the content it stays within."
        defaultOpen
      >
        <ChannelLineupEditor channelId={channelId} revision={revision} lineup={lineup} />
        <ChannelPolicyFields policy={policy} onChange={onPolicyChange} show="scope" />
        {/* `scope.series` narrows the channel to specific shows. It sits under the scope
            fields because it is the same question ("what may play?") at a coarser grain than
            era/ceiling — and beside the lineup editor whose search picker it reuses. */}
        <ChannelSeriesScope policy={policy} onChange={onPolicyChange} />
        {/* `scope.collections` — the same "what may play?" question against the operator's own
            media-server shelves. Renders nothing when no library is configured. */}
        <ChannelCollectionsScope policy={policy} onChange={onPolicyChange} />
      </Block>

      <Block title="How it's ordered" hint="The order and spacing programs play in.">
        <ChannelPolicyFields policy={policy} onChange={onPolicyChange} show="ordering" strategy={strategy} />
      </Block>

      <Block
        title="When it changes"
        hint="Play different things at different times: weekend marathons, holiday blocks, day-parts."
      >
        {vocabulary ? (
          <>
            {/* ⚠ The rules editor edits the DRAFT, not the saved policy (§12 — the third
                review-before-apply surface). Rules resolve first-match-by-priority, so an
                intermediate state is a different schedule; inline-saving each step would
                reconcile half-finished rule sets to Tunarr. */}
            <ChannelRulesEditor
              policy={rules.draft}
              onChange={rules.setDraft}
              lineupKeys={lineupKeys}
              vocabulary={vocabulary}
            />

            {/* Apply / Discard — offered only when the draft differs from what is saved, and
                deliberately the same shape as the filler sandbox's bar: two drafts on one page
                that commit differently would be two things to learn. */}
            {rules.isDirty && (
              <div className="flex items-center gap-2 border-border border-t pt-4">
                <Button variant="suggest" size="sm" disabled={rules.isApplying} onClick={rules.apply}>
                  {rules.isApplying ? "Applying…" : "Apply rules"}
                </Button>
                <Button variant="ghost" size="sm" disabled={rules.isApplying} onClick={rules.discard}>
                  Discard
                </Button>
                <span className="ml-auto text-muted-foreground text-xs">Unsaved changes</span>
              </div>
            )}

            {/* ⚠ Seasonal stays SEAMLESS: it writes `policy.seasonal`, a self-contained field,
                not `policy.rules`. §12 scopes the draft to the interdependent surface, so a
                block that happens to sit nearby does not inherit an Apply click it never
                needed. It reads the SAVED policy for the same reason. */}
            {/* Seasonal (§6) belongs to "when it changes" on the longest clock of the three:
                the rules above switch by wall-clock time of day/week, auto-curate below grows
                the lineup over weeks, and this one follows the calendar year. It shares the
                vocabulary the rules editor already fetched — the holiday ids come from the
                same BE-authored list, so the picker cannot offer a holiday the engine does
                not know. */}
            <div className="border-border/60 border-t pt-4">
              <ChannelSeasonal policy={policy} onChange={onPolicyChange} vocabulary={vocabulary} />
            </div>
          </>
        ) : (
          <p className="text-muted-foreground text-sm">Loading rule options…</p>
        )}

        {/* Auto-curate (§8.2) — the same "when does this change" question on a much slower
            clock. The rules above move titles around WITHIN the lineup by wall-clock time;
            this decides whether the lineup itself grows as the library does. Sharing the block
            keeps both answers in one place instead of on a settings tab that never existed
            (§12). */}
        <div className="border-border/60 border-t pt-4">
          <ChannelAutoCurate policy={policy} onChange={onPolicyChange} intentBacked={Boolean(intentRef)} />
        </div>
      </Block>

      {/* One shared preview: time-travel the schedule to see exactly what airs — and which rule
          wins — at any moment. Verifies the deck, the ordering, AND the rules above. */}
      <CollapsibleSection
        title="Preview schedule"
        description="Check what airs at a specific time and which rule wins."
      >
        <ChannelCyclePreview
          channelId={channelId}
          lineupKeys={lineupKeys}
          draftPolicy={rules.isDirty ? rules.draft : undefined}
        />
      </CollapsibleSection>
    </div>
  );
};

export { ChannelProgramming };
