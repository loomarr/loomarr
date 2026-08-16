import { formatMmSs } from "@loomarr/core/format";
import { Scissors, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
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
      <section aria-label={`Split ${clip.name} into clips`} className="flex flex-col gap-4 p-4">
        <div className="flex items-start gap-3">
          <TriangleAlert className="mt-0.5 size-5 shrink-0 text-signal" aria-hidden />
          <div className="min-w-0 flex-1">
            <h2 className="font-semibold text-lg">Split “{clip.name}” into clips?</h2>
            <p className="mt-1 text-muted-foreground text-sm">
              Loomarr will decode the whole {formatMmSs(clip.durationMs ?? 0)} recording looking for the
              adverts inside it. That takes <strong>several minutes</strong> and uses the same hardware as
              playback, so a live channel may stutter while it runs.
            </p>
            {/* The reassurance is as load-bearing as the warning: an operator who thinks this
                might destroy their compilation will not run it at all, and 45 reels stay parked. */}
            <p className="mt-2 text-muted-foreground text-sm">
              Nothing enters the catalog yet — you review the proposed cuts afterwards, and this recording is
              left exactly as it is.
            </p>
          </div>
        </div>

        <div className="flex justify-end gap-2">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button size="sm" onClick={onConfirm}>
            <Scissors aria-hidden />
            Find the clips
          </Button>
        </div>
      </section>
    </Card>
  );
};

export { ConfirmSplitDialog };
