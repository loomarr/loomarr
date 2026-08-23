import type { ChannelFitDTO } from "@loomarr/api/models/channelFitDTO";
import { Ban, RotateCcw } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";
import { cn } from "@/lib/utils";
import type { ChannelOverridePickerProps } from "./channel-override-picker.type";

// ChannelOverridePicker — which channels use this clip (§10, V35 item 1.7).
//
// ⚠ **Three states, not two, and this is the whole design.** The domain has `pinned` (forced in
// ahead of the ladder), `excluded` (never used — and it WINS over pinned), and neither: the
// default, where the clip competes on the ladder like every other clip. The mock draws one
// checkbox per channel, and a naive checkbox collapses "not pinned" into "excluded", silently
// blocking clips the operator merely did not tick.
//
// So the checkbox only MEANS anything once a channel is overridden. Automatic is the default
// and "Back to automatic" returns to it. Ratified with the maintainer before building, because
// getting it wrong silently blocks an operator's catalog.
//
// ⚠ **The fit note's WORDING lives here, not on the server.** The API sends a reason CODE (the
// §11 refusal-code precedent): a server sentence cannot be translated and cannot link to the
// setting that caused it.

// What each ladder rung means for THIS clip on THIS channel. Phrased about the clip, because
// that is what the operator is deciding about — the channel-wide meter says it the other way.
const LEVEL_NOTE: Record<string, string> = {
  exact: "matches this channel exactly",
  widened: "close enough — same decade",
  audience: "fits the audience, wrong era",
};

// Why a clip would never be picked. Each names the thing an operator can go and change.
const REASON_NOTE: Record<string, string> = {
  kind: "this channel doesn't use this kind of clip",
  duration: "too long or short for this channel's breaks",
  quality: "below this channel's quality floor",
  category: "not one of this channel's categories",
  audience: "wrong audience for this channel",
  excluded: "you've blocked it here",
};

// ⚠ A PINNED clip has no rung — a pin is placed ahead of the ladder — so `level` is
// `bumper_card` with no reason, which would otherwise read as "won't be picked". The pin state
// is checked FIRST for exactly that reason.
const fitNote = (fit: ChannelFitDTO): string => {
  if (fit.excluded) return REASON_NOTE.excluded ?? "blocked here";
  if (fit.pinned) return "always played here";
  if (fit.reason) return REASON_NOTE[fit.reason] ?? "won't be picked here";
  return LEVEL_NOTE[fit.level] ?? "may be picked here";
};

// A channel is OVERRIDDEN when the operator has said something explicit about this clip.
// Neither flag = automatic, which is the state "Back to automatic" restores.
const isOverridden = (fit: ChannelFitDTO): boolean => fit.pinned || fit.excluded;

const ChannelOverridePicker = ({
  clipName,
  channels,
  onSet,
  onReset,
  busyChannelId,
  loading,
  error,
  className,
}: ChannelOverridePickerProps) => (
  <div className={cn("flex flex-col gap-3", className)}>
    {/* The mock's `modeNote`: what the checkboxes mean, said once, above them. Without it a
        row of unticked boxes reads as "blocked everywhere" rather than "decided automatically".
        ⚠ **This copy said "untick to block it" until V51f, and was describing the bug.** The
        checkbox now releases a pin rather than blocking, and blocking has its own button — so the
        sentence had to move with the behaviour. Caught by LOOKING at a regenerated visual
        baseline: every unit test here asserts the write, and none of them read the instructions. */}
    <Caption>
      Loomarr picks channels for {clipName} automatically. Tick a channel to always play it there, or use
      Block to keep it off that channel. A pin takes priority over automatic rotation, so it may repeat inside
      the cooldown or reduce variety. Untick to hand the choice back to Loomarr.
    </Caption>

    {error && <p className="text-onair-300 text-sm">{error}</p>}
    {loading && <Caption>Checking each channel…</Caption>}

    {!loading && channels.length === 0 && <Caption>No channels yet — this clip has nowhere to play.</Caption>}

    <ul className="flex flex-col gap-1">
      {channels.map((fit) => {
        const overridden = isOverridden(fit);
        const busy = busyChannelId === fit.channelId;
        return (
          <li
            key={fit.channelId}
            className="flex flex-wrap items-center gap-3 rounded-md border border-border px-3 py-2"
          >
            <input
              type="checkbox"
              // ⚠ Ticked means PINNED, never merely "not blocked". An automatic channel shows
              // UNTICKED, which is why the mode note above has to explain that unticked is not
              // the same as blocked until the row is overridden.
              checked={fit.pinned}
              disabled={busy}
              // ⚠ **Unticking returns the row to AUTOMATIC; it does not block (§10 V51f).**
              // This wrote `{pinned: false, excluded: true}`, so releasing a pin silently moved
              // the clip to "never play here" — the exact failure the ⚠ at the top of this file
              // warns about, committed by the file carrying the warning. Blocking is a decision,
              // and it now has its own button; a checkbox cannot express three states, so the two
              // that are not "pinned" must be reachable some other way.
              onChange={(e) =>
                e.target.checked
                  ? onSet(fit.channelId, { pinned: true, excluded: false })
                  : onReset(fit.channelId)
              }
              className="size-4 shrink-0 accent-signal"
              // The visible channel name is in the next cell; naming the action here means a
              // screen-reader user does not hear a list of identical "checkbox"es.
              aria-label={`Always play ${clipName} on ${fit.name}`}
            />
            <span className="w-8 shrink-0 text-right font-mono text-muted-foreground text-xs">
              {fit.number}
            </span>
            <span className="min-w-0 flex-1 truncate text-sm">{fit.name}</span>

            {/* ⚠ The note is the reason this endpoint exists. "Won't be picked" with no cause
                sends an operator hunting through channel settings for a rule they cannot see. */}
            <Caption className="shrink-0">{fitNote(fit)}</Caption>

            {overridden ? (
              <>
                <Badge variant={fit.excluded ? "caution" : "signal"}>
                  {fit.excluded ? "blocked" : "always"}
                </Badge>
                {/* The mock's "Back to automatic" — the ONLY way back to the third state, since
                    a checkbox cannot express it. */}
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  onClick={() => onReset(fit.channelId)}
                  aria-label={`Back to automatic for ${fit.name}`}
                >
                  <RotateCcw className="size-3.5" aria-hidden />
                  Automatic
                </Button>
              </>
            ) : (
              <>
                <Badge variant="neutral">automatic</Badge>
                {/* ⚠ Blocking is now an EXPLICIT action rather than a side effect of unticking.
                    It is the destructive half of the three states — "never play this here" — and
                    the one an operator is most likely to trigger by accident when it rides on a
                    checkbox they were using to un-pin. */}
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  onClick={() => onSet(fit.channelId, { pinned: false, excluded: true })}
                  aria-label={`Block ${clipName} on ${fit.name}`}
                >
                  <Ban className="size-3.5" aria-hidden />
                  Block
                </Button>
              </>
            )}
          </li>
        );
      })}
    </ul>
  </div>
);

export { ChannelOverridePicker };
