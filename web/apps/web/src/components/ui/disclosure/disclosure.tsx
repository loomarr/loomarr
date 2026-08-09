import { Collapsible } from "@base-ui/react/collapsible";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib";
import type { DisclosurePanelProps, DisclosureProps, DisclosureTriggerProps } from "./disclosure.type";

// Disclosure — a reveal whose trigger is a DISCRETE chevron, not the whole header (§5.1c, Layer 1).
//
// ⚠ **`CollapsibleSection` cannot do this job, and the reason is structural rather than stylistic.**
// It wraps its entire header in one `<button>`, which is correct for a bordered section whose
// header is nothing but a title — and invalid the moment the row also holds a Switch, a Preview
// button, or a link. Nested interactive content inside a button is not a lint preference; it is
// unreachable by keyboard and undefined in the accessibility tree. So a dense row gets a chevron
// it can place beside its own controls, and the row owns its layout.
//
// It is deliberately NOT an Accordion: nothing here enforces single-open exclusivity, because the
// surfaces that need this (a queue of clips, a tree of sources) must be able to open several at
// once to compare them.
//
// ⚠ Closed content leaves the accessibility tree rather than merely being clipped —
// `hiddenUntilFound` renders `hidden="until-found"`, so a screen reader does not walk forty
// collapsed panels while find-in-page still reaches the text and opens the right one.
const DisclosureRoot = ({
  open,
  defaultOpen = false,
  onOpenChange,
  children,
  className,
}: DisclosureProps) => (
  <Collapsible.Root
    open={open}
    defaultOpen={defaultOpen}
    // Base UI passes (open, eventDetails); the public contract is (open) => void, so the second
    // argument is dropped deliberately rather than widened into the exported type.
    onOpenChange={(next) => onOpenChange?.(next)}
    render={<div className={cn(className)} />}
  >
    {children}
  </Collapsible.Root>
);

// ⚠ The chevron follows the trigger's own `data-panel-open` rather than a React boolean. The
// state lives in the primitive, so reading it back out to compute a class would be a second
// source of truth for one fact — and the two can disagree for a frame during a transition.
const DisclosureTrigger = ({ label, className }: DisclosureTriggerProps) => (
  <Collapsible.Trigger
    aria-label={label}
    className={cn(
      "group inline-flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-static-800 hover:text-foreground",
      className,
    )}
  >
    <ChevronDown className="size-4 transition-transform group-data-[panel-open]:rotate-180" aria-hidden />
  </Collapsible.Trigger>
);

// The reveal: grid 0fr→1fr so the body slides open with no fixed height (styles.css).
//
// ⚠ `.reveal` keys off `data-open`, which Base UI emits VALUELESS when open — styles.css matches
// `[data-open=""]` alongside the `[data-open="true"]` a React boolean produces. It deliberately
// does not match on attribute presence alone, because React renders `data-open={false}` as the
// STRING "false".
const DisclosurePanel = ({ children, className }: DisclosurePanelProps) => (
  <Collapsible.Panel hiddenUntilFound className="reveal">
    <div className="reveal-inner">
      <div className={cn(className)}>{children}</div>
    </div>
  </Collapsible.Panel>
);

// ⚠ Attached as properties rather than exported as three names, and that is load-bearing for more
// than ergonomics: `story-coverage.test.ts` enumerates the barrel's runtime function exports, so
// three exports would demand three story files for one component. One compound, one story.
const Disclosure = Object.assign(DisclosureRoot, { Panel: DisclosurePanel, Trigger: DisclosureTrigger });

export { Disclosure };
