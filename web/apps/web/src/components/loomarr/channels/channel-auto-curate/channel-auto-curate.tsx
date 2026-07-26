import { Checkbox, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
import { FieldHelp } from "../../feedback";
import type { ChannelAutoCurateProps } from "./channel-auto-curate.type";

// ChannelAutoCurate — the per-channel auto-curate opt-in (programming-design.md §8.2).
//
// ORPHANED until now, and for a structural reason worth keeping written down: **the opt-in IS
// the object's presence.** `policy.autoCurate` is a `*AutoCurate` — nil means off, and a
// zero-value (non-nil) object means "opted in, inherit both global thresholds". So there is no
// boolean field to bind a checkbox to; the checkbox has to CONSTRUCT `{}` or DELETE the object.
// A generic policy field editor could never reach it, which is exactly why nothing did.
//
// The opt-out is safe because PATCH /v1/channels/{id} replaces the policy wholesale
// (schedule.MergeFromOperator: `out := incoming`, only `Applied` is force-preserved). Sending a
// policy with `autoCurate` absent therefore genuinely clears it — this is NOT a sparse patch
// where an omitted key would read as "unchanged" and silently strand the channel opted in.

// What the checkbox turns on, in the operator's words. Deliberately NOT "keep this channel
// updated": per §8.2 a channel that has NOT opted in still gets re-curation *proposals* in the
// approval queue — re-curation runs either way. The opt-in only decides whether net-new
// acquisitions are auto-approved (bounded by the quality bar + title cap) instead of waiting
// for an admin. Labelling it "keep updated" would imply opting out freezes the channel, which
// is false and would make the safer choice look like the useless one.
const OPT_IN_LABEL = "Add new titles without asking";

const OPT_IN_HELP =
  "Loomarr re-checks this channel's intent against your library on a schedule. Off, it proposes " +
  "changes and waits for approval. On, in-library matches are added and net-new titles are " +
  "requested automatically — still through the approval gate, and still bounded by the limits below.";

// Both overrides are `0 = inherit the global default` on the wire (int64, omitempty). So a
// blank field must send 0, never undefined: undefined is dropped by omitempty and the previous
// value would survive. Same trap the runtimeMax control documents in ChannelPolicyFields — and
// the opposite of separation.blockMax, where 0 would mean a real, different thing.
const overrideOrInherit = (raw: string): number => {
  const trimmed = raw.trim();
  if (trimmed === "") return 0;
  return Math.max(0, Math.round(Number(trimmed)));
};

const ChannelAutoCurate = ({ policy, onChange, intentBacked = true, className }: ChannelAutoCurateProps) => {
  const auto = policy.autoCurate;
  const enabled = auto !== undefined;

  // Construct on tick, delete on untick. `rest` drops the key entirely rather than setting it
  // undefined — both serialize the same through JSON, but dropping it keeps the object honest
  // for the equality checks callers do on the draft policy.
  const toggle = (next: boolean) => {
    if (next) {
      onChange({ ...policy, autoCurate: {} });
      return;
    }
    const { autoCurate: _removed, ...rest } = policy;
    onChange(rest);
  };

  return (
    <div className={cn("flex flex-col gap-3", className)}>
      <div className="flex items-start gap-2.5">
        <Checkbox
          id="policy-auto-curate"
          className="mt-0.5"
          checked={enabled}
          disabled={!intentBacked}
          aria-describedby="policy-auto-curate-hint"
          onChange={(e) => toggle(e.target.checked)}
        />
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-1.5">
            <Label htmlFor="policy-auto-curate">{OPT_IN_LABEL}</Label>
            <FieldHelp label={OPT_IN_LABEL}>{OPT_IN_HELP}</FieldHelp>
          </div>
          {/* The hint tracks the CURRENT state — it is the sentence that says what is happening
              now, not a static description of the setting. (An earlier version always read "Off,
              new titles wait for your approval", which flatly contradicted a ticked box; the
              unit tests passed and the story screenshot caught it.) */}
          <p id="policy-auto-curate-hint" className="text-muted-foreground text-sm">
            {!intentBacked
              ? "Only for channels built from an intent — this one was made by hand, so there is no intent to re-check."
              : enabled
                ? "New titles that fit are added on their own, within the limits below."
                : "Off, new titles wait for your approval."}
          </p>
        </div>
      </div>

      {/* The two overrides appear only once opted in: they modify a behaviour that is
          otherwise not running, and showing disabled boxes for a feature that is off reads as
          broken rather than inapplicable. Blank = inherit the fleet-wide default. */}
      {enabled && (
        <div className="flex flex-wrap gap-3 pl-7">
          <div className="flex flex-col gap-1">
            <Label htmlFor="policy-auto-curate-score" className="text-muted-foreground text-xs">
              Quality bar
            </Label>
            <Input
              id="policy-auto-curate-score"
              className="w-28"
              type="number"
              min={0}
              max={100}
              defaultValue={auto?.minScorePct ? auto.minScorePct : ""}
              placeholder="Default"
              onBlur={(e) => {
                const next = Math.min(100, overrideOrInherit(e.target.value));
                if (next === (auto?.minScorePct ?? 0)) return;
                onChange({ ...policy, autoCurate: { ...auto, minScorePct: next } });
              }}
            />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="policy-auto-curate-cap" className="text-muted-foreground text-xs">
              Title cap
            </Label>
            <Input
              id="policy-auto-curate-cap"
              className="w-28"
              type="number"
              min={0}
              defaultValue={auto?.maxTitles ? auto.maxTitles : ""}
              placeholder="Default"
              onBlur={(e) => {
                const next = overrideOrInherit(e.target.value);
                if (next === (auto?.maxTitles ?? 0)) return;
                onChange({ ...policy, autoCurate: { ...auto, maxTitles: next } });
              }}
            />
          </div>
        </div>
      )}
    </div>
  );
};

export { ChannelAutoCurate };
