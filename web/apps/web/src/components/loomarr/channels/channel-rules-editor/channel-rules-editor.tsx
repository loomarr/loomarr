import {
  DndContext,
  type DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { SchedulingRule, Vocabulary } from "@loomarr/api";
import { GripVertical, Plus, X } from "lucide-react";
import { useId, useState } from "react";
import {
  Button,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui";
import { cn } from "@/lib";
import type { ChannelRulesEditorProps } from "./channel-rules-editor.type";
import { computeLabel } from "./label";
import {
  howOptions,
  lowerHow,
  lowerWhat,
  lowerWhen,
  tokenForHow,
  tokenForWhat,
  tokenForWhen,
  whatStaticOptions,
  whenOptions,
} from "./presets";

// ChannelRulesEditor — the "Programming rules" authoring surface (programming-design
// §6.5/§6.6/§8.1): an ordered list of (WHEN, WHAT, HOW) rule chips. Authoring is
// TOKEN-BASED, never raw predicates — the editor composes from the SAME closed
// vocabulary the LLM uses (presets.ts, mirroring internal/schedule/presets.go), so a
// hand-authored rule and an LLM-authored one lower to byte-identical SchedulingRule
// fields. The backend re-validates on save regardless (unknown tokens dropped, window
// clamped, audience stricter-only) — this table is a UX convenience, not the source of
// truth.
//
// PRIORITY IS LIST ORDER: §8.1 says "reordering the chips rewrites each rule's
// Priority" — so this editor doesn't let the user type a number at all. On every
// add/reorder/edit/remove the whole `rules` array is rebuilt and each rule's `priority`
// is re-derived from its position: `priority = (rules.length - index) * 10`. The top
// rule (index 0) always gets the highest multiple, so dragging a rule to the top always
// makes it win overlaps — a simple, legible scheme (10, 20, 30, … from the bottom up)
// that doesn't depend on remembering each token's own "default priority" once the user
// has taken over ordering by hand.
const priorityFor = (index: number, total: number): number => (total - index) * 10;

// WINDOW_FULL_WIRE is the wire form of schedule.WindowFull (-1) — "the whole run, don't
// truncate a binge" (§6.5). ChannelPolicy/SchedulingRule.window is a Go-duration STRING on
// the wire, and Duration(-1).String() is "-1ns"; the backend's UnmarshalJSON parses "-1ns"
// straight back to WindowFull. A marathon MUST use this, NOT "0s" — "0s" is Duration(0),
// which resolveWindow reads as "inherit the channel/global window" (i.e. it WOULD truncate),
// the opposite of a binge. Keeping this a named constant (not a bare string) so its meaning
// is legible and it can't be mistaken for "no window". Mirrors schedule.WindowFull.
const WINDOW_FULL_WIRE = "-1ns";

// A rule as the editor edits it: the three authoring tokens (source of truth for the
// pickers) plus the id/label/window carried through from the SchedulingRule. `whatDisplay`
// caches the series title (from lineupKeys) so the computed label can show "Predator"
// instead of a bare provisioning key.
interface RuleDraft {
  id: string;
  whenToken: string;
  whatToken: string;
  whatDisplay?: string;
  howToken: string;
}

const draftFromRule = (
  rule: SchedulingRule,
  lineupKeys: { key: string; title: string }[],
  vocab: Vocabulary,
): RuleDraft => {
  const whatToken = tokenForWhat(vocab.what, rule.what);
  const seriesKey = rule.what?.series?.[0];
  const whatDisplay = seriesKey ? lineupKeys.find((l) => l.key === seriesKey)?.title : undefined;
  return {
    id: rule.id ?? crypto.randomUUID(),
    whenToken: tokenForWhen(vocab.when, rule.when),
    whatToken,
    whatDisplay,
    howToken: tokenForHow(vocab.how, rule.how),
  };
};

const ruleFromDraft = (draft: RuleDraft, index: number, total: number, vocab: Vocabulary): SchedulingRule => {
  const when = lowerWhen(vocab.when, draft.whenToken);
  const what = lowerWhat(vocab.what, draft.whatToken);
  const how = lowerHow(vocab.how, draft.howToken);
  return {
    id: draft.id,
    label: computeLabel(
      {
        whenToken: draft.whenToken,
        whatToken: draft.whatToken,
        whatDisplay: draft.whatDisplay,
        howToken: draft.howToken,
      },
      vocab,
    ),
    priority: priorityFor(index, total),
    ...(when ? { when: when.predicate } : {}),
    ...(what ? { what: what.scope } : {}),
    ...(how ? { how: how.ordering, ...(how.window === "full" ? { window: WINDOW_FULL_WIRE } : {}) } : {}),
  };
};

// rebuild — rewrites the whole rules array from drafts, re-deriving priority from
// position every time (the one function every mutation funnels through).
const rebuild = (drafts: RuleDraft[], vocab: Vocabulary): SchedulingRule[] =>
  drafts.map((d, i) => ruleFromDraft(d, i, drafts.length, vocab));

// One rule row: drag handle, WHEN/WHAT/HOW selects, computed label, remove.
const RuleRow = ({
  draft,
  disabled,
  lineupKeys,
  vocab,
  onChange,
  onRemove,
}: {
  draft: RuleDraft;
  disabled: boolean;
  lineupKeys: { key: string; title: string }[];
  vocab: Vocabulary;
  onChange: (next: RuleDraft) => void;
  onRemove: () => void;
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: draft.id,
  });
  const rowId = useId();

  const style = { transform: CSS.Transform.toString(transform), transition };

  const label = computeLabel(
    {
      whenToken: draft.whenToken,
      whatToken: draft.whatToken,
      whatDisplay: draft.whatDisplay,
      howToken: draft.howToken,
    },
    vocab,
  );

  const whatOptions = [
    ...whatStaticOptions(vocab.what),
    ...lineupKeys.map((l) => ({ value: `series:${l.key}`, label: `Series: ${l.title}` })),
  ];

  return (
    <li
      ref={setNodeRef}
      style={style}
      className={cn(
        "flex flex-col gap-2 rounded-md border border-border bg-card px-3 py-2.5",
        isDragging && "z-10 opacity-70 shadow-md",
      )}
    >
      <div className="flex items-center gap-2">
        <button
          type="button"
          {...attributes}
          {...listeners}
          disabled={disabled}
          aria-label={`Reorder ${label}`}
          className="flex size-7 shrink-0 cursor-grab items-center justify-center rounded text-static-400 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing disabled:cursor-not-allowed disabled:opacity-50"
        >
          <GripVertical className="size-4" aria-hidden />
        </button>
        <span className="min-w-0 flex-1 truncate font-medium text-sm">{label}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="size-7 shrink-0"
              disabled={disabled}
              aria-label={`Remove ${label}`}
              onClick={onRemove}
            >
              <X aria-hidden />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Remove this rule</TooltipContent>
        </Tooltip>
      </div>

      <div className="grid grid-cols-1 gap-2 pl-9 sm:grid-cols-3">
        <Select value={draft.whenToken} onValueChange={(v) => onChange({ ...draft, whenToken: v })}>
          <SelectTrigger aria-label={`When for ${label}`} id={`${rowId}-when`}>
            <SelectValue placeholder="When…" />
          </SelectTrigger>
          <SelectContent>
            {whenOptions(vocab.when).map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={draft.whatToken}
          onValueChange={(v) => {
            const seriesKey = v.startsWith("series:") ? v.slice("series:".length) : undefined;
            const whatDisplay = seriesKey ? lineupKeys.find((l) => l.key === seriesKey)?.title : undefined;
            onChange({ ...draft, whatToken: v, whatDisplay });
          }}
        >
          <SelectTrigger aria-label={`What for ${label}`} id={`${rowId}-what`}>
            <SelectValue placeholder="What…" />
          </SelectTrigger>
          <SelectContent>
            {whatOptions.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={draft.howToken} onValueChange={(v) => onChange({ ...draft, howToken: v })}>
          <SelectTrigger aria-label={`How for ${label}`} id={`${rowId}-how`}>
            <SelectValue placeholder="How…" />
          </SelectTrigger>
          <SelectContent>
            {howOptions(vocab.how).map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </li>
  );
};

const ChannelRulesEditor = ({
  policy,
  onChange,
  lineupKeys = [],
  vocabulary,
  className,
}: ChannelRulesEditorProps) => {
  const rules = policy.rules ?? [];
  const [drafts, setDrafts] = useState<RuleDraft[]>(() =>
    rules.map((r) => draftFromRule(r, lineupKeys, vocabulary)),
  );

  // Keep local drafts in sync when the server hands back a genuinely different rule set
  // (another session edited it, or the LLM proposed a starter set) — same fingerprint-
  // adoption pattern as useChannelLineup: compare the identity SET+ORDER (rule ids), not
  // the array reference (a new `[]`/object literal on every incidental parent re-render),
  // so an in-flight local edit isn't mistaken for a remote change and clobbered by it. Our
  // own commits round-trip the SAME draft ids (ruleFromDraft carries `draft.id` straight
  // through), so a save-then-echo-back leaves the fingerprint unchanged and never re-syncs.
  const idsFingerprint = rules.map((r) => r.id ?? "").join(" ");
  const [adoptedFingerprint, setAdoptedFingerprint] = useState(idsFingerprint);
  if (idsFingerprint !== adoptedFingerprint) {
    setDrafts(rules.map((r) => draftFromRule(r, lineupKeys, vocabulary)));
    setAdoptedFingerprint(idsFingerprint);
  }

  const commit = (next: RuleDraft[]) => {
    setDrafts(next);
    onChange({ ...policy, rules: rebuild(next, vocabulary) });
  };

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const onDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const from = drafts.findIndex((d) => d.id === active.id);
    const to = drafts.findIndex((d) => d.id === over.id);
    if (from === -1 || to === -1) return;
    commit(arrayMove(drafts, from, to));
  };

  const addRule = () => {
    const draft: RuleDraft = {
      id: crypto.randomUUID(),
      whenToken: "weekend",
      whatToken: "all",
      howToken: "syndication",
    };
    commit([draft, ...drafts]);
  };

  return (
    <section className={cn("flex flex-col gap-3", className)}>
      <div>
        <h3 className="font-semibold text-base">Wall-clock rules</h3>
        <p className="text-muted-foreground text-sm">
          Play different things at different times: weekend marathons, holiday programming, day-parts. The top
          rule wins when more than one matches a moment; drag to reprioritize.
        </p>
      </div>

      {drafts.length === 0 ? (
        <div className="rounded-md border border-border border-dashed px-3 py-6 text-center">
          <p className="text-muted-foreground text-sm">
            No rules yet. This channel always follows its base policy above.
          </p>
          <Button variant="outline" size="sm" className="mt-3" onClick={addRule}>
            <Plus aria-hidden />
            Add rule
          </Button>
        </div>
      ) : (
        <>
          <DndContext sensors={sensors} onDragEnd={onDragEnd}>
            <SortableContext items={drafts.map((d) => d.id)} strategy={verticalListSortingStrategy}>
              <ul className="flex flex-col gap-2">
                {drafts.map((draft) => (
                  <RuleRow
                    key={draft.id}
                    draft={draft}
                    disabled={false}
                    lineupKeys={lineupKeys}
                    vocab={vocabulary}
                    onChange={(next) => commit(drafts.map((d) => (d.id === draft.id ? next : d)))}
                    onRemove={() => commit(drafts.filter((d) => d.id !== draft.id))}
                  />
                ))}
              </ul>
            </SortableContext>
          </DndContext>
          <Button variant="outline" size="sm" className="self-start" onClick={addRule}>
            <Plus aria-hidden />
            Add rule
          </Button>
        </>
      )}
    </section>
  );
};

export { ChannelRulesEditor };
