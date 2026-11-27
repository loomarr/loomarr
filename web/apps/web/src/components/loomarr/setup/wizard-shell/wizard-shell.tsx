import {
  Action,
  BrandLockup,
  Card,
  Grid,
  Icon,
  icons,
  PressableRow,
  Stack,
  Text,
} from "@loomarr/design-system";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { IdleSnow } from "../../shell";
import type { WizardShellProps, WizardStepStatus } from "./wizard-shell.type";

// WizardShell — the operator first-run frame (§13, config-design §6). Two columns, like
// the app's own console: a fixed left rail (the compact brand + a VERTICAL step list of
// where you are and what's left) and a scrollable content column with the current step's
// form and its nav. The rail is resume-safe by construction — it renders whatever status
// the caller derived from GET /v1/setup/status, so a refresh loses nothing — and skipped
// steps render neutral, never red (§6): an optional step you passed on is not a failure.
const MARKER: Record<WizardStepStatus, { backgroundColor?: string; borderColor: string }> = {
  done: { backgroundColor: "$stateSuccessSurface", borderColor: "$stateSuccess" },
  // Where you are is neutral (#1909 Q1): its row already takes the elevated fill, so the marker is
  // a foreground ring and number. Amber is focus or the primary action.
  current: { borderColor: "$contentPrimary" },
  pending: { borderColor: "$borderDecorative" },
  skipped: { borderColor: "$borderDecorative" },
};

// The rail casts onto the content column beside it.
const RAIL_SHADOW = "4px 0 16px -6px rgba(0,0,0,0.6)";

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
}: WizardShellProps) => {
  // The rail stacks above the content on a phone, so its hairline moves from its side to its foot.
  const phoneWidth = usePhoneWidth();
  return (
    <Grid
      backgroundColor="$surfaceCanvas"
      className={className}
      columns={{ base: 1, md: ["17rem", "1fr"] }}
      // A prop, not `min-h-screen`: a View's own `min-height: 0` outranks the utility.
      minHeight="100vh"
      position="relative"
      render="main"
    >
      {/* Behind everything: the snow is a positioned layer, so content siblings need a stacking
        order above it (zIndex 10). A position:absolute layer paints OVER position:static content
        regardless of DOM order. */}
      <IdleSnow />

      {/* Left rail: brand + vertical step list. Fixed width on desktop; on narrow screens it
        stacks above the content (the whole wizard is desktop-first, but never broken). */}
      <Stack
        backgroundColor="$surfaceRaised"
        borderColor="$borderDecorative"
        borderBottomWidth={phoneWidth ? 1 : 0}
        borderRightWidth={phoneWidth ? 0 : 1}
        gap="section"
        paddingHorizontal={24}
        paddingVertical={32}
        position="relative"
        render="aside"
        style={{ boxShadow: RAIL_SHADOW }}
        zIndex={10}
      >
        <Stack gap="1">
          {/* The lockup hugs the rail's left edge, in line with the step markers below. */}
          <Stack align="start">
            <BrandLockup size="small" />
          </Stack>
          <Stack marginTop={8}>
            <Text as="p" textRole="heading">
              Let's get you on the air
            </Text>
          </Stack>
          <Text as="p" textRole="mono" tone="secondary">
            First-run setup · saved as you go
          </Text>
        </Stack>

        <Stack aria-label="Setup steps" gap="1" render="ol">
          {steps.map((step, i) => {
            const status = statusById[step.id] ?? "pending";
            const isCurrent = step.id === currentId;
            const subItems = isCurrent ? step.subItems : undefined;
            // A reached step is a way back to it. One not yet reached stays text: clicking ahead
            // would land on the frontier anyway (resolveStep), which reads as a broken link.
            const canGo = !isCurrent && onStep !== undefined && reachable?.has(step.id) === true;
            const content = (
              <>
                <Stack
                  align="center"
                  {...MARKER[status]}
                  borderRadius={999}
                  borderWidth={1}
                  flexShrink={0}
                  height={24}
                  justify="center"
                  render="span"
                  width={24}
                >
                  {status === "done" ? (
                    <Icon decorative glyph={icons.selected} size="compact" tone={MARKER_TONE[status]} />
                  ) : (
                    <Text as="span" strong={status === "current"} textRole="mono" tone={MARKER_TONE[status]}>
                      {i + 1}
                    </Text>
                  )}
                </Stack>
                <Text
                  as="span"
                  textRole={isCurrent ? "cardLabel" : "compact"}
                  tone={isCurrent ? "primary" : "secondary"}
                >
                  {step.title}
                </Text>
                {status === "skipped" && (
                  <Stack marginLeft="auto" render="span">
                    <Text as="span" textRole="hint" tone="secondary">
                      skipped
                    </Text>
                  </Stack>
                )}
              </>
            );
            return (
              <li key={step.id} aria-current={isCurrent ? "step" : undefined}>
                {/* The row's own inset (its 2px border and 10px padding) bleeds outward, so the step
                  markers line up with the heading above and only the fill reaches past it. */}
                <Stack marginHorizontal={-12}>
                  {canGo ? (
                    <PressableRow onPress={() => onStep?.(step.id)}>{content}</PressableRow>
                  ) : (
                    <Stack
                      align="center"
                      backgroundColor={isCurrent ? "surfaceElevated" : undefined}
                      direction="row"
                      gap="control"
                      paddingHorizontal={12}
                      paddingVertical={10}
                    >
                      {content}
                    </Stack>
                  )}
                </Stack>

                {subItems && subItems.length > 0 && (
                  // Second-level connections: an indented list that appears under the current
                  // step. The vertical guide line ties them to their parent.
                  <Stack
                    borderColor="$borderDecorative"
                    borderLeftWidth={1}
                    gap="0.5"
                    marginLeft={25.6}
                    marginTop={4}
                    paddingLeft={12}
                    render="ul"
                  >
                    {subItems.map((sub) => {
                      const active = sub.id === activeSubItem;
                      return (
                        <li key={sub.id}>
                          <PressableRow
                            aria-current={active ? "true" : undefined}
                            onPress={() => onSubItem?.(sub.id)}
                            selected={active}
                          >
                            <Text
                              as="span"
                              strong={active}
                              textRole="compact"
                              tone={active ? "primary" : "secondary"}
                            >
                              {sub.label}
                            </Text>
                          </PressableRow>
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
      <Stack flexShrink={1} minWidth={0} position="relative" zIndex={10}>
        <Stack
          flexBasis="0%"
          flexGrow={1}
          flexShrink={1}
          gap="6"
          marginHorizontal="auto"
          maxWidth={672}
          paddingHorizontal={24}
          paddingVertical={{ base: 32, md: 48 }}
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
    </Grid>
  );
};

export { WizardShell };
