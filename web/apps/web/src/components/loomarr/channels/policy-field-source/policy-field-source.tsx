import { Action, Badge, Stack } from "@loomarr/design-system";
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
  <Stack align="center" className={className} direction="row" gap="inline" wrap>
    {/* An override is neutral with a foreground label and a control hairline (#1909 Q5): the word
        carries the meaning, and amber stays for focus and the primary action. The `override` tone
        draws the hairline without making this badge taller than the Default one. */}
    <Badge shape="square" tone={overridden ? "override" : "neutral"}>
      {overridden ? overrideLabel : defaultLabel}
    </Badge>
    {overridden ? (
      // A dozen identical "Reset to default" buttons would be indistinguishable in a screen
      // reader's button list or to voice control, so the name carries the field while the
      // visible text stays the mock's (and still starts the name).
      <Action
        accessibilityLabel={`Reset to default (${label})`}
        layout="text"
        onPress={onReset}
        variant="ghost"
      >
        Reset to default
      </Action>
    ) : null}
  </Stack>
);

export { PolicyFieldSource };
