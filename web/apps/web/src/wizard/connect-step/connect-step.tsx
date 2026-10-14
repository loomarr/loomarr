import { humanizeSettingKey } from "@loomarr/core/format";
import { Loader2 } from "lucide-react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ChecklistItem } from "@/components/loomarr/setup/checklist-item";
import { Button } from "@/components/ui/button";
import type { ConnectStepProps } from "./connect-step.type";

// The shape shared by the two one-click wiring steps (§13 steps 3 and 5): explain what
// the click does, show the driving check, and do it. Both are **idempotent and never
// silent** (§6) — the button stays available once green so re-running is a normal act,
// and the check (not the click) is what reports success, because the BE's own probe is
// the only honest source of "this actually worked".
const ConnectStep = ({ check, cta, onConnect, isPending = false, error, children }: ConnectStepProps) => {
  const connected = check?.ok === true;

  return (
    <div className="flex flex-col gap-4">
      <div className="text-muted-foreground text-sm">{children}</div>

      {error != null && <ErrorState error={error} />}

      {check && (
        <ChecklistItem
          name={humanizeSettingKey(check.name)}
          status={isPending ? "running" : check.ok ? "pass" : "fail"}
          hint={check.hint}
          docHref={check.docHref}
        />
      )}

      <Button
        variant={connected ? "outline" : "default"}
        className="w-fit"
        onClick={onConnect}
        disabled={isPending}
      >
        {isPending && <Loader2 className="animate-spin" aria-hidden />}
        {isPending ? "Working…" : connected ? "Run again" : cta}
      </Button>
    </div>
  );
};

export { ConnectStep };
