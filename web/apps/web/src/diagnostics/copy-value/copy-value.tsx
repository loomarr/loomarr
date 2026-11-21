import { Action } from "@loomarr/design-system";
import { useCopied } from "@/lib/use-copied";

// One identifier on a diagnostics page (a run, channel, job or request id) that copies itself.
// The value is the label, and the name holds it too (WCAG 2.5.3); the icon confirms the copy,
// and only when the copy really wrote it.
const CopyValue = ({ label, value }: { label: string; value: string }) => {
  const { copied, copy } = useCopied();
  return (
    <Action
      accessibilityLabel={`Copy ${label}: ${value}`}
      fitLabel
      icon={copied ? "success" : "copy"}
      onPress={() => void copy(value)}
      variant="ghost"
    >
      {value}
    </Action>
  );
};

export { CopyValue };
