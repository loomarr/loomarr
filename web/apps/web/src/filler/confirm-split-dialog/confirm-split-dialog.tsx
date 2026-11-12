import { formatMmSs } from "@loomarr/core/format";
import { Action, Card, Icon, icons, Stack } from "@loomarr/design-system";
import type { ConfirmSplitDialogProps } from "./confirm-split-dialog.type";

// ConfirmSplitDialog — the confirmation "Split into clips" never had (§10 V54 A8).
//
// ⚠ **The button fired `POST /v1/filler/split` on the first click, with one `<p role="status">`
// line as the only feedback.** That call starts a full-decode ffmpeg pass — chapters →
// blackdetect/silencedetect → transcript rescue — which is MINUTES on a long recording, and it
// competes with playout for the same GPU the §9.1 admission control exists to protect. An
// operator scanning a grid of cards could start several by mis-clicking and have no way to tell
// they had, because the card's only change is one disabled button.
//
// ⚠ **What it does NOT need to warn about is losing anything.** Detection writes a PROPOSAL and
// nothing else; review is not optional (§10 V34), so no clip enters or leaves the catalog here.
// Saying so is the point — a confirmation that only says "are you sure?" makes the operator guess
// at the stakes, and the honest stakes are *time and GPU*, not data. A dialog that overstates the
// risk trains people to click through it.
const ConfirmSplitDialog = ({ clip, onConfirm, onClose }: ConfirmSplitDialogProps) => {
  if (!clip) return null;

  return (
    <Card>
      <Stack aria-label={`Split ${clip.name} into clips`} className="p-4" gap="4" render="section">
        <Stack align="start" direction="row" gap="control">
          <Stack className="mt-0.5" direction="row" render="span">
            <Icon decorative glyph={icons.alert} size="default" tone="primary" />
          </Stack>
          <div className="min-w-0 flex-1">
            <h2 className="text-lg font-semibold">Split “{clip.name}” into clips?</h2>
            <p className="text-muted-foreground mt-1 text-sm">
              Loomarr will decode the whole {formatMmSs(clip.durationMs ?? 0)} recording looking for the
              adverts inside it. That takes <strong>several minutes</strong> and uses the same hardware as
              playback, so a live channel may stutter while it runs.
            </p>
            {/* The reassurance is as load-bearing as the warning: an operator who thinks this
                might destroy their compilation will not run it at all, and 45 reels stay parked. */}
            <p className="text-muted-foreground mt-2 text-sm">
              Nothing enters the catalog yet — you review the proposed cuts afterwards, and this recording is
              left exactly as it is.
            </p>
          </div>
        </Stack>

        <Stack direction="row" gap="inline" justify="end">
          <Action onPress={onClose} variant="ghost">
            Cancel
          </Action>
          <Action icon="split" onPress={onConfirm}>
            Find the clips
          </Action>
        </Stack>
      </Stack>
    </Card>
  );
};

export { ConfirmSplitDialog };
