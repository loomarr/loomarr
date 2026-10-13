import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { PolicyFieldSourceProps } from "./policy-field-source.type";

// PolicyFieldSource — where a Programming field's value comes from (#1817 item 4): a channel
// override or the default, plus a "Reset to default" that writes the field's inherit sentinel.
// Reset stays visible but disabled on a default field, so every field's row has the same shape
// and the badge alone is not the only signal. `hideResetWhenDefault` is for the auto-curate
// opt-in, whose default is a plain unticked box.
const PolicyFieldSource = ({
  overridden,
  onReset,
  label,
  overrideLabel = "Channel override",
  defaultLabel = "Default",
  hideResetWhenDefault,
  className,
}: PolicyFieldSourceProps) => (
  <div className={cn("flex flex-wrap items-center gap-2", className)}>
    <Badge variant={overridden ? "signal" : "neutral"}>{overridden ? overrideLabel : defaultLabel}</Badge>
    {overridden || !hideResetWhenDefault ? (
      <Button
        type="button"
        variant="link"
        className="h-auto p-0 text-xs underline"
        disabled={!overridden}
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
