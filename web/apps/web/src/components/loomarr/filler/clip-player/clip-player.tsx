import { clipMediaURL } from "@loomarr/core";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { Button, VideoPlayer } from "@/components/ui";
import { cn } from "@/lib";
import type { ClipPlayerProps } from "./clip-player.type";

// ClipPlayer — watch one catalog clip in a modal (V39).
//
// Deliberately thin: everything about PLAYING is the `VideoPlayer` primitive's job, and
// everything here is about the dialog and about clips. That split is what lets the channel-watch
// surface the mock sketches reuse the same player for a live stream without inheriting
// `ClipDTO`.
//
// ⚠ Built on Radix's Dialog PRIMITIVE rather than the app's `DialogContent` wrapper. The wrapper
// hardcodes `max-w-md` and `p-6` and renders its own close button in the top-right — this needs a
// wide, unpadded surface, and the top-right corner is where the clip's title goes. The primitive
// still supplies the focus trap, Escape handling, scroll-lock and aria-modal, which is the part
// that actually matters.
const ClipPlayer = ({ clip, onClose, className }: ClipPlayerProps) => (
  <DialogPrimitive.Root
    open={clip !== null}
    onOpenChange={(next) => {
      if (!next) onClose();
    }}
  >
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 fixed inset-0 z-50 bg-black/80 data-[state=closed]:animate-out data-[state=open]:animate-in" />
      <DialogPrimitive.Content
        className={cn(
          "data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 fixed top-1/2 left-1/2 z-50 w-full max-w-3xl -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-lg border border-border bg-static-950 shadow-lg focus:outline-none data-[state=closed]:animate-out data-[state=open]:animate-in",
          className,
        )}
      >
        {/* ⚠ Radix REQUIRES a title (it warns loudly without one) and it is what names the dialog
            for a screen reader. It is sr-only here because the visible title is rendered inside
            the player's own overlay, where the maintainer's spec puts it — announcing both would
            read the clip's name twice. */}
        <DialogPrimitive.Title className="sr-only">
          {clip ? `Playing ${clip.name}` : "Clip player"}
        </DialogPrimitive.Title>

        {/* ⚠ Mounted ONLY with a clip, so closing the dialog unmounts the <video> and stops the
            download. Rendering it unconditionally would leave a paused element holding a
            connection open for as long as the page lived. */}
        {clip && (
          <VideoPlayer
            // ⚠ `key` on the clip's identity: without it, reopening the dialog on a DIFFERENT
            // clip reuses the same element and React keeps its state, so the new clip inherits
            // the previous one's playhead. The key forces a fresh player per clip.
            key={clip.hash}
            src={clipMediaURL(clip.hash)}
            title={clip.name}
            // The operator clicked a play button to get here, so the gesture that permits
            // autoplay has already happened.
            autoPlay
            leading={
              // `asChild` so Radix's Close behaviour composes onto the app's Button rather than
              // onto a hand-styled <button> that would have to re-derive the cursor and the focus
              // ring itself.
              <DialogPrimitive.Close asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Close player"
                  className="size-8 bg-static-900/80 text-static-100 hover:bg-static-900 hover:text-white focus-visible:ring-offset-0"
                >
                  <X aria-hidden />
                </Button>
              </DialogPrimitive.Close>
            }
          />
        )}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  </DialogPrimitive.Root>
);

export { ClipPlayer };
