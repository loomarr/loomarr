import { Check, Loader2 } from "lucide-react";
import { Button, Card, CardContent } from "@/components/ui";
import { cn } from "@/lib";
import { BrandLockup, TvStatic } from "../../shell";
import type { WizardShellProps, WizardStepStatus } from "./wizard-shell.type";

// WizardShell — the operator first-run frame (§13, config-design §6). Two columns, like
// the app's own console: a fixed left rail (the compact brand + a VERTICAL step list of
// where you are and what's left) and a scrollable content column with the current step's
// form and its nav. The rail is resume-safe by construction — it renders whatever status
// the caller derived from GET /v1/setup/status, so a refresh loses nothing — and skipped
// steps render neutral, never red (§6): an optional step you passed on is not a failure.
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
  activeSubItem,
  onSubItem,
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
  <main
    className={cn("relative grid min-h-screen grid-cols-1 bg-background md:grid-cols-[17rem_1fr]", className)}
  >
    {/* Behind everything: the static is a positioned layer, so content siblings need an
        explicit stacking context above it (relative z-10) — otherwise a position:absolute
        layer paints OVER position:static content regardless of DOM order. */}
    <TvStatic className="z-0" />

    {/* Left rail: brand + vertical step list. Fixed width on desktop; on narrow screens it
        stacks above the content (the whole wizard is desktop-first, but never broken). */}
    <aside className="relative z-10 flex flex-col gap-6 border-border border-b bg-card px-6 py-8 shadow-[4px_0_16px_-6px_rgba(0,0,0,0.6)] md:border-r md:border-b-0">
      <div className="flex flex-col gap-1">
        <BrandLockup variant="compact" />
        <p className="mt-2 font-semibold text-base">Let's get you on the air</p>
        <p className="font-mono text-muted-foreground text-xs">First-run setup · saved as you go</p>
      </div>

      <ol aria-label="Setup steps" className="flex flex-col gap-1">
        {steps.map((step, i) => {
          const status = statusById[step.id] ?? "pending";
          const isCurrent = step.id === currentId;
          const subItems = isCurrent ? step.subItems : undefined;
          return (
            <li key={step.id} aria-current={isCurrent ? "step" : undefined}>
              <div
                className={cn(
                  "-mx-2 flex items-center gap-2.5 rounded-md px-2 py-1.5",
                  isCurrent && "bg-static-800",
                )}
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
                  className={cn(
                    "text-sm",
                    isCurrent ? "font-medium text-foreground" : "text-muted-foreground",
                  )}
                >
                  {step.title}
                </span>
                {status === "skipped" && <span className="ml-auto text-static-400 text-xs">skipped</span>}
              </div>

              {subItems && subItems.length > 0 && (
                // Second-level connections: an indented list that appears under the current
                // step. The vertical guide line ties them to their parent.
                <ul className="mt-1 ml-[1.6rem] flex flex-col gap-0.5 border-border border-l pl-3">
                  {subItems.map((sub) => {
                    const active = sub.id === activeSubItem;
                    return (
                      <li key={sub.id}>
                        <button
                          type="button"
                          onClick={() => onSubItem?.(sub.id)}
                          aria-current={active ? "true" : undefined}
                          className={cn(
                            "w-full cursor-pointer rounded px-2 py-1 text-left text-sm transition-colors",
                            active
                              ? "font-medium text-signal"
                              : "text-muted-foreground hover:text-foreground",
                          )}
                        >
                          {sub.label}
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
            </li>
          );
        })}
      </ol>
    </aside>

    {/* Content column: the current step. Stacked above the static like the rail. */}
    <div className="relative z-10 flex min-w-0 flex-col">
      <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col gap-6 px-6 py-8 md:py-12">
        <div className="flex flex-col gap-1">
          <h1 className="font-semibold text-2xl">{title}</h1>
          {description && <p className="text-muted-foreground text-sm">{description}</p>}
        </div>

        <Card>
          <CardContent className="flex flex-col gap-5 p-6">{children}</CardContent>
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
    </div>
  </main>
);

export { WizardShell };
