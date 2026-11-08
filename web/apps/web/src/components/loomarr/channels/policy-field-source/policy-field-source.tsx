import { Badge } from "@loomarr/design-system";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { PolicyFieldSourceProps } from "./policy-field-source.type";

// PolicyFieldSource — where a Programming field's value comes from (#1817 item 4): a channel
// override or the default, plus a "Reset to default" that writes the field's inherit sentinel.
// Reset shows only beside an override: beside the Default badge it would have nothing to do (#2744).
const PolicyFieldSource = ({
  overridden,
  onReset,
  label,
  overrideLabel = "Channel override",
  defaultLabel = "Default",
  className,
}: PolicyFieldSourceProps) => (
  <div className={cn("flex flex-wrap items-center gap-2", className)}>
    {/* An override is neutral with a foreground label and a control hairline (#1909 Q5): the word
        carries the meaning, and amber stays for focus and the primary action. The `override` tone
        draws the hairline without making this badge taller than the Default one. */}
    <Badge shape="square" tone={overridden ? "override" : "neutral"}>
      {overridden ? overrideLabel : defaultLabel}
    </Badge>
    {overridden ? (
      <Button
        type="button"
        variant="link"
        className="h-auto p-0 text-xs underline"
        // 16 px tall as drawn: a touch screen gets the 24 px tap area (styles.css, #1785).
        data-touch-target
        onClick={onReset}
      >
        {/* A dozen identical "Reset to default" buttons would be indistinguishable in a
            screen reader's button list or to voice control, so the name carries the field
            while the visible text stays the mock's (and still starts the name). */}
        Reset to default <span className="sr-only">({label})</span>
      </Button>
    ) : null}
  </div>
);

export { PolicyFieldSource };
