import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";
import type {
  DialogContentProps,
  DialogDescriptionProps,
  DialogSectionProps,
  DialogTitleProps,
} from "./dialog.type";

// The app's modal primitive, on Base UI Dialog (design §14 — same family as Select/Menu).
// The primitive gives the fiddly a11y for free: focus trap, Escape to close, aria-modal +
// labelling, scroll-lock. Styling matches the app system — a `popover`/`card` surface with the
// signal focus ring — so a dialog reads as the same design language as the rest of the UI.
const Dialog = DialogPrimitive.Root;
const DialogTrigger = DialogPrimitive.Trigger;
const DialogClose = DialogPrimitive.Close;

// ⚠ Radix's `Overlay` is Base UI's `Backdrop`, and it is a SIBLING of the popup inside the portal.
// The old version carried `data-[state=open]:animate-in` / `fade-in-0` classes on both this and the
// content; they are deleted rather than translated because `tailwindcss-animate` is not a
// dependency here (see styles.css), so they compiled to nothing. Deleting them must not move a
// pixel — which makes the visual baselines the check on that claim.
const DialogContent = ({ className, children, ...props }: DialogContentProps) => (
  <DialogPrimitive.Portal>
    <DialogPrimitive.Backdrop className="fixed inset-0 z-50 bg-black/60" />
    <DialogPrimitive.Popup
      className={cn(
        "fixed top-1/2 left-1/2 z-50 flex w-full max-w-md -translate-x-1/2 -translate-y-1/2 flex-col gap-4 rounded-lg border border-border bg-card p-6 shadow-lg focus:outline-none",
        className,
      )}
      {...props}
    >
      {children}
      <DialogPrimitive.Close
        className="absolute top-4 right-4 rounded-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-label="Close"
      >
        <X className="size-4" aria-hidden />
      </DialogPrimitive.Close>
    </DialogPrimitive.Popup>
  </DialogPrimitive.Portal>
);

const DialogHeader = ({ className, ...props }: DialogSectionProps) => (
  <div className={cn("flex flex-col gap-1.5", className)} {...props} />
);

const DialogFooter = ({ className, ...props }: DialogSectionProps) => (
  <div className={cn("flex justify-end gap-2", className)} {...props} />
);

const DialogTitle = ({ className, ...props }: DialogTitleProps) => (
  <DialogPrimitive.Title className={cn("font-semibold text-lg", className)} {...props} />
);

const DialogDescription = ({ className, ...props }: DialogDescriptionProps) => (
  <DialogPrimitive.Description className={cn("text-muted-foreground text-sm", className)} {...props} />
);

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
};
