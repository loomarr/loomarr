import { Action } from "@loomarr/design-system";
import { useState } from "react";

// One identifier on a diagnostics page (a run, channel, job or request id) that copies itself.
// The value is the label, and the name holds it too (WCAG 2.5.3); the icon confirms the copy.
const CopyValue = ({ label, value }: { label: string; value: string }) => {
  const [copied, setCopied] = useState(false);
  return (
    <Action
      accessibilityLabel={`Copy ${label}: ${value}`}
      fitLabel
      icon={copied ? "success" : "copy"}
      onPress={() =>
        void navigator.clipboard.writeText(value).then(() => {
          setCopied(true);
          window.setTimeout(() => setCopied(false), 1_500);
        })
      }
      variant="ghost"
    >
      {value}
    </Action>
  );
};

export { CopyValue };
