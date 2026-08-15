import { Info } from "lucide-react";
import { useId } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { FieldHelpProps } from "./field-help.type";

// FieldHelp — a small (i) icon that shows a field's guidance on hover/focus, instead of a
// permanent muted paragraph under every control. This is what lets the settings forms breathe:
// the help is there when you want it, out of the way when you don't. Uses the app-wide Tooltip
// (TooltipProvider is mounted at the root, __root.tsx) so no local provider is needed.
//
// ⚠ THE `sr-only` COPY IS LOAD-BEARING — do not "de-duplicate" it against the tooltip.
//
// Base UI's Tooltip is deliberately visual-only: its Popup carries no `role="tooltip"` and its
// Trigger gets no `aria-describedby`, and its docs say so outright — "Tooltips are visual-only
// elements and are not a replacement for labeling the trigger." Radix wired that association for
// us, so the swap silently removed it. Everywhere else in the app that costs nothing, because the
// trigger's `aria-label` already restates the tooltip ("Sign out" → "Sign out"). HERE it does not:
// the content is the field's DOCUMENTATION (`entry.doc` for every setting), which appears nowhere
// else in the DOM. A screen-reader user would have heard "About Ordering, button" and nothing more.
//
// So the description is declared explicitly and permanently, and `aria-describedby` points at it.
// That is what Radix did under the hood; it is written out here because the primitive no longer
// does it. This file's comment previously claimed the SR user "hears the same guidance" — under
// Base UI that claim would have been false, which is exactly why it is spelled out now.
const FieldHelp = ({ children, label, describedById, className }: FieldHelpProps) => {
  const ownId = useId();
  // Reuse the consumer's existing description when there is one (SettingField renders the doc for
  // its control already); only mint and render a copy when this is the sole carrier of the text.
  const descriptionId = describedById ?? ownId;

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <button
            type="button"
            aria-label={`About ${label}`}
            aria-describedby={descriptionId}
            className={cn(
              "inline-flex size-4 shrink-0 cursor-help items-center justify-center rounded-full text-static-400 transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              className,
            )}
          />
        }
      >
        <Info className="size-3.5" aria-hidden />
        {!describedById && (
          <span id={descriptionId} className="sr-only">
            {children}
          </span>
        )}
      </TooltipTrigger>
      <TooltipContent>{children}</TooltipContent>
    </Tooltip>
  );
};

export { FieldHelp };
