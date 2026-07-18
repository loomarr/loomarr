import { Check, Loader2, Radio } from "lucide-react";
import { Button, Card, CardContent } from "@/components/ui";
import { cn } from "@/lib";
import type { WizardShellProps, WizardStepStatus } from "./wizard-shell.type";

// WizardShell — the operator first-run frame (§13, config-design §6). A numbered rail
// showing where you are and what's left, the current step's form, and the nav. The rail
// is resume-safe by construction: it renders whatever status the caller derived from
// GET /v1/setup/status, so a browser refresh loses nothing. Skipped steps render
// neutral, never red (§6) — an optional step you passed on is not a failure.
const MARKER: Record<WizardStepStatus, string> = {
  done: "border-lock bg-lock-tint-15 text-lock",
  current: "border-signal bg-signal-tint-15 text-signal",
  pending: "border-border text-static-400",
  skipped: "border-border text-static-400",
};

const WizardShell = ({
  steps,
  currentId,
  statusById,
  title,
  description,
  children,
  onBack,
  onNext,
  onSkip,
  nextLabel = "Continue",
  nextDisabled = false,
  busy = false,
  className,
}: WizardShellProps) => (
  <main className={cn("flex min-h-screen flex-col items-center bg-background px-6 py-10", className)}>
    <div className="flex w-full max-w-2xl flex-col gap-8">
      <div className="flex items-center gap-2">
        <Radio className="size-5 text-signal" aria-hidden />
        <span className="font-mono font-semibold text-md tracking-tight">Loomarr</span>
        <span className="text-muted-foreground text-sm">· first-run setup</span>
      </div>

      <ol aria-label="Setup steps" className="flex flex-wrap gap-x-5 gap-y-2">
        {steps.map((step, i) => {
          const status = statusById[step.id] ?? "pending";
          const isCurrent = step.id === currentId;
          return (
            <li
              key={step.id}
              aria-current={isCurrent ? "step" : undefined}
              className="flex items-center gap-2"
            >
              <span
                className={cn(
                  "flex size-6 shrink-0 items-center justify-center rounded-full border font-mono text-xs",
                  MARKER[status],
                )}
              >
                {status === "done" ? <Check className="size-3.5" aria-hidden /> : i + 1}
              </span>
              <span
                className={cn("text-sm", isCurrent ? "font-medium text-foreground" : "text-muted-foreground")}
              >
                {step.title}
              </span>
              {status === "skipped" && <span className="text-static-400 text-xs">skipped</span>}
            </li>
          );
        })}
      </ol>

      <Card>
        <CardContent className="flex flex-col gap-5 p-6">
          <div className="flex flex-col gap-1">
            <h1 className="font-semibold text-lg">{title}</h1>
            {description && <p className="text-muted-foreground text-sm">{description}</p>}
          </div>
          {children}
        </CardContent>
      </Card>

      <div className="flex items-center gap-2">
        {onBack && (
          <Button variant="ghost" onClick={onBack} disabled={busy}>
            Back
          </Button>
        )}
        <div className="ml-auto flex items-center gap-2">
          {onSkip && (
            <Button variant="outline" onClick={onSkip} disabled={busy}>
              Skip for now
            </Button>
          )}
          {onNext && (
            <Button onClick={onNext} disabled={nextDisabled || busy}>
              {busy && <Loader2 className="animate-spin" aria-hidden />}
              {nextLabel}
            </Button>
          )}
        </div>
      </div>
    </div>
  </main>
);

export { WizardShell };
