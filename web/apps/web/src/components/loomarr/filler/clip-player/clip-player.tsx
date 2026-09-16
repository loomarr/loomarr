import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { clipMediaURL } from "@loomarr/core/clip-thumb";
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { VideoPlayer } from "@/components/ui/video-player";
import { cn } from "@/lib/utils";
import type { ClipPlayerProps } from "./clip-player.type";

// ClipPlayer — watch one catalog clip in a modal (V39).
//
// Deliberately thin: everything about PLAYING is the `VideoPlayer` primitive's job, and
// everything here is about the dialog and about clips. That split is what lets the channel-watch
// surface the mock sketches reuse the same player for a live stream without inheriting
// `ClipDTO`.
//
// ⚠ Built on the Dialog PRIMITIVE rather than the app's `DialogContent` wrapper. The wrapper
// hardcodes `max-w-md` and `p-6` and renders its own close button in the top-right — this needs a
// wide, unpadded surface, and the top-right corner is where the clip's title goes. The primitive
// still supplies the focus trap, Escape handling, scroll-lock and aria-modal, which is the part
// that actually matters.
const ClipPlayer = ({ clip, onClose, onPlaybackStart, className }: ClipPlayerProps) => (
  <DialogPrimitive.Root
    open={clip !== null}
    onOpenChange={(next) => {
      if (!next) onClose();
    }}
  >
    <DialogPrimitive.Portal>
      <DialogPrimitive.Backdrop className="fixed inset-0 z-50 bg-black/80" />
      <DialogPrimitive.Popup
        className={cn(
          "fixed top-1/2 left-1/2 z-50 w-full max-w-3xl -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-lg border border-border bg-static-950 shadow-lg focus:outline-none",
          className,
        )}
      >
        {/* ⚠ The title is what NAMES the dialog for a screen reader — keep it even though Base UI,
            unlike Radix, does not warn when it is missing. It is sr-only here because the visible
            title is rendered inside the player's own overlay, where the maintainer's spec puts it —
            announcing both would read the clip's name twice. */}
        <DialogPrimitive.Title className="sr-only">
          {clip ? `Playing ${clip.name}` : "Clip player"}
        </DialogPrimitive.Title>

        {/* ⚠ Mounted ONLY with a clip, so closing the dialog unmounts the <video> and stops the
            download. Rendering it unconditionally would leave a paused element holding a
            connection open for as long as the page lived. */}
        {clip && (
          <ClipPreview
            clip={clip}
            onPlaybackStart={onPlaybackStart}
            leading={
              <DialogPrimitive.Close
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Close player"
                    className="size-8 bg-static-900/80 text-static-100 hover:bg-static-900 hover:text-white focus-visible:ring-offset-0"
                  />
                }
              >
                <X aria-hidden />
              </DialogPrimitive.Close>
            }
          />
        )}
      </DialogPrimitive.Popup>
    </DialogPrimitive.Portal>
  </DialogPrimitive.Root>
);

// The same exact-media player inside a Sheet, without a second modal/focus trap.
const ClipPreview = ({
  clip,
  onPlaybackStart,
  leading,
}: {
  clip: NonNullable<ClipPlayerProps["clip"]>;
  onPlaybackStart?: ClipPlayerProps["onPlaybackStart"];
  leading?: import("react").ReactNode;
}) => (
  <VideoPlayer
    key={clip.hash}
    src={clipMediaURL(clip.hash)}
    title={clip.name}
    autoPlay
    onPlaybackStart={onPlaybackStart}
    leading={leading}
  />
);

export { ClipPlayer, ClipPreview };
