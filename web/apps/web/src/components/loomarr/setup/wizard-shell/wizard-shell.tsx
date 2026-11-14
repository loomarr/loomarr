import { Action, Card, Icon, icons, Stack, Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import { BrandLockup, TvStatic } from "../../shell";
import type { WizardShellProps, WizardStepStatus } from "./wizard-shell.type";

// WizardShell — the operator first-run frame (§13, config-design §6). Two columns, like
// the app's own console: a fixed left rail (the compact brand + a VERTICAL step list of
// where you are and what's left) and a scrollable content column with the current step's
// form and its nav. The rail is resume-safe by construction — it renders whatever status
// the caller derived from GET /v1/setup/status, so a refresh loses nothing — and skipped
// steps render neutral, never red (§6): an optional step you passed on is not a failure.
const MARKER: Record<WizardStepStatus, string> = {
  done: "border-lock bg-lock-tint-15",
  // Where you are is neutral (#1909 Q1): its row already takes the elevated fill, so the marker is
  // a foreground ring and number. Amber is focus or the primary action.
  current: "border-foreground",
  pending: "border-border",
  skipped: "border-border",
};

const MARKER_TONE: Record<WizardStepStatus, "primary" | "secondary" | "success"> = {
  done: "success",
  current: "primary",
  pending: "secondary",
  skipped: "secondary",
};

const WizardShell = ({
  steps,
  currentId,
  statusById,
  activeSubItem,
  onSubItem,
  reachable,
  onStep,
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
    className={cn("bg-background relative grid min-h-screen grid-cols-1 md:grid-cols-[17rem_1fr]", className)}
  >
    {/* Behind everything: the static is a positioned layer, so content siblings need an
        explicit stacking context above it (relative z-10) — otherwise a position:absolute
        layer paints OVER position:static content regardless of DOM order. */}
    <TvStatic className="z-0" />

    {/* Left rail: brand + vertical step list. Fixed width on desktop; on narrow screens it
        stacks above the content (the whole wizard is desktop-first, but never broken). */}
    <Stack
      className="border-border bg-card relative z-10 border-b shadow-[4px_0_16px_-6px_rgba(0,0,0,0.6)] md:border-r md:border-b-0"
      gap="section"
      paddingHorizontal={24}
      paddingVertical={32}
      render="aside"
    >
      <Stack gap="1">
        <BrandLockup variant="compact" />
        <Text as="p" className="mt-2" textRole="heading">
          Let's get you on the air
        </Text>
        <Text as="p" textRole="mono" tone="secondary">
          First-run setup · saved as you go
        </Text>
      </Stack>

      <Stack aria-label="Setup steps" gap="1" render="ol">
        {steps.map((step, i) => {
          const status = statusById[step.id] ?? "pending";
          const isCurrent = step.id === currentId;
          const subItems = isCurrent ? step.subItems : undefined;
          const row = cn(
            "-mx-2 flex items-center gap-2.5 rounded-md px-2 py-1.5",
            isCurrent && "bg-static-800",
          );
          // A reached step is a way back to it. One not yet reached stays text: clicking ahead
          // would land on the frontier anyway (resolveStep), which reads as a broken link.
          const canGo = !isCurrent && onStep !== undefined && reachable?.has(step.id) === true;
          const content = (
            <>
              <span
                className={cn(
                  "flex size-6 shrink-0 items-center justify-center rounded-full border",
                  MARKER[status],
                )}
              >
                {status === "done" ? (
                  <Icon decorative glyph={icons.selected} size="compact" tone={MARKER_TONE[status]} />
                ) : (
                  <Text as="span" strong={status === "current"} textRole="mono" tone={MARKER_TONE[status]}>
                    {i + 1}
                  </Text>
                )}
              </span>
              <Text
                as="span"
                textRole={isCurrent ? "cardLabel" : "compact"}
                tone={isCurrent ? "primary" : "secondary"}
              >
                {step.title}
              </Text>
              {status === "skipped" && (
                <Text as="span" className="ml-auto" textRole="hint" tone="secondary">
                  skipped
                </Text>
              )}
            </>
          );
          return (
            <li key={step.id} aria-current={isCurrent ? "step" : undefined}>
              {canGo ? (
                <button
                  type="button"
                  onClick={() => onStep?.(step.id)}
                  className={cn(row, "w-[calc(100%+1rem)] cursor-pointer text-left")}
                >
                  {content}
                </button>
              ) : (
                <div className={row}>{content}</div>
              )}

              {subItems && subItems.length > 0 && (
                // Second-level connections: an indented list that appears under the current
                // step. The vertical guide line ties them to their parent.
                <Stack
                  className="border-border mt-1 ml-[1.6rem] border-l"
                  gap="0.5"
                  paddingLeft={12}
                  render="ul"
                >
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
                              ? "text-foreground font-semibold"
                              : "text-muted-foreground hover:text-foreground",
                          )}
                        >
                          {sub.label}
                        </button>
                      </li>
                    );
                  })}
                </Stack>
              )}
            </li>
          );
        })}
      </Stack>
    </Stack>

    {/* Content column: the current step. Stacked above the static like the rail. */}
    <Stack className="relative z-10" flexShrink={1} minWidth={0}>
      <Stack
        className="max-w-2xl px-6 py-8 md:py-12"
        flexBasis="0%"
        flexGrow={1}
        flexShrink={1}
        gap="6"
        marginHorizontal="auto"
        width="100%"
      >
        <Stack gap="1">
          <Text as="h1" strong textRole="display">
            {title}
          </Text>
          {description && (
            <Text as="p" textRole="compact" tone="secondary">
              {description}
            </Text>
          )}
        </Stack>

        <Card gap={20} padding={24}>
          {children}
        </Card>

        <Stack align="center" direction="row" gap="inline">
          {onBack && (
            <Action disabled={busy} onPress={onBack} variant="ghost">
              Back
            </Action>
          )}
          <Stack align="center" direction="row" gap="inline" marginLeft="auto">
            {onSkip && (
              <Action disabled={busy} onPress={onSkip} variant="outline">
                Skip for now
              </Action>
            )}
            {onNext && (
              <Action disabled={nextDisabled || busy} onPress={onNext}>
                {nextLabel}
              </Action>
            )}
          </Stack>
        </Stack>
      </Stack>
    </Stack>
  </main>
);

export { WizardShell };
