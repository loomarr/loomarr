import { Action, Tooltip } from "@loomarr/design-system";
import { useId } from "react";
import { cn } from "@/lib/utils";
import type { FieldHelpProps } from "./field-help.type";

// FieldHelp — a small (i) icon that shows a field's guidance on hover/focus, instead of a
// permanent muted paragraph under every control. This is what lets the settings forms breathe:
// the help is there when you want it, out of the way when you don't.
//
// ⚠ THE `sr-only` COPY IS LOAD-BEARING — do not "de-duplicate" it against the tooltip.
//
// Base UI's Tooltip is deliberately visual-only: its Popup carries no `role="tooltip"` and its
// Trigger gets no `aria-describedby`, and its docs say so outright — "Tooltips are visual-only
// elements and are not a replacement for labeling the trigger." Everywhere else in the app that
// costs nothing, because the trigger's `aria-label` already restates the tooltip ("Sign out" →
// "Sign out"). HERE it does not: the content is the field's DOCUMENTATION (`entry.detail` for a
// setting), which appears nowhere else in the DOM. Without the wiring below, a screen-reader user
// hears "About Ordering, button" and nothing more.
//
// So the description is declared explicitly and permanently, and `aria-describedby` points at it.
// The primitive does not do it, so it is written out here; do not assume the SR user "hears the
// same guidance" from the tooltip alone.
const FieldHelp = ({ children, label, describedById, className }: FieldHelpProps) => {
  const ownId = useId();
  // Reuse the consumer's existing description when there is one (SettingField renders the doc for
  // its control already); only mint and render a copy when this is the sole carrier of the text.
  const descriptionId = describedById ?? ownId;

  return (
    <span className={cn("inline-flex shrink-0", className)}>
      <Tooltip content={children}>
        <Action
          aria-describedby={descriptionId}
          compact
          icon="info"
          layout="icon"
          variant="ghost"
        >{`About ${label}`}</Action>
      </Tooltip>
      {!describedById && (
        <span id={descriptionId} className="sr-only">
          {children}
        </span>
      )}
    </span>
  );
};

export { FieldHelp };
