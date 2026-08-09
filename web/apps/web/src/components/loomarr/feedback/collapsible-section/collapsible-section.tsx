import { Collapsible } from "@base-ui/react/collapsible";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib";
import type { CollapsibleSectionProps } from "./collapsible-section.type";

// CollapsibleSection — the app's accordion section (§12/§13): a bordered card whose body
// slides open/closed from a clickable header. Extracted from the wizard's checklist blocks
// (the proven pattern) so the channel-detail workbench can present its editors calm — each
// section collapsed until you open the one you're tuning, instead of a wall of expanded forms.
//
// ⚠ **The ANIMATION stayed hand-rolled on purpose (V50c).** Base UI's Collapsible measures the
// panel (`scrollHeight` → `--collapsible-panel-height`) so an author can transition `height`.
// This keeps the `.reveal` grid-rows 0fr→1fr trick instead (styles.css): height-agnostic, so
// nothing has to be measured and a body that changes size mid-open cannot desync from a stale
// measurement. Reduced-motion is unaffected either way — styles.css freezes transitions globally
// under the media query, not per component. So the primitive owns STATE and SEMANTICS here; the
// stylesheet still owns motion.
//
// What the port actually buys, since the old version's a11y was already correct (a real
// <button aria-expanded aria-controls>, body clipped rather than unmounted):
//
//   • `hiddenUntilFound` — the browser's find-in-page can now reach text inside a CLOSED
//     section and open it. The old version clipped the body with `overflow:hidden`, which
//     leaves the text findable by nothing. On a workbench whose sections are collapsed by
//     default that is the difference between Ctrl+F working and silently missing content.
//   • The controlled/uncontrolled fork, the id wiring between trigger and panel, and the
//     transition bookkeeping stop being ours to keep correct.
const CollapsibleSection = ({
  title,
  description,
  icon,
  trailing,
  defaultOpen = false,
  open,
  onOpenChange,
  children,
  className,
}: CollapsibleSectionProps) => (
  <Collapsible.Root
    // ⚠ `open` is forwarded as-is, INCLUDING undefined — that is what selects uncontrolled mode.
    // Coercing it (`open ?? false`) would silently pin every uncontrolled section shut, which is
    // the failure mode this component's own prop docs warn about.
    open={open}
    defaultOpen={defaultOpen}
    // Base UI passes (open, eventDetails); this component's contract is (open) => void, so the
    // second argument is dropped deliberately rather than widened into the public type.
    onOpenChange={(next) => onOpenChange?.(next)}
    render={<section className={cn("overflow-hidden rounded-lg border border-border", className)} />}
  >
    <Collapsible.Trigger className="group flex w-full cursor-pointer items-center gap-3 px-5 py-4 text-left transition-colors hover:bg-static-800">
      {icon && <span className="flex shrink-0 items-center text-muted-foreground">{icon}</span>}
      <span className="min-w-0">
        <span className="block font-semibold text-lg leading-tight">{title}</span>
        {description && <span className="mt-0.5 block text-muted-foreground text-sm">{description}</span>}
      </span>
      {trailing && <span className="ml-auto shrink-0">{trailing}</span>}
      {/* The chevron follows the trigger's own `data-panel-open` rather than a React boolean —
          the state lives in the primitive now, so reading it back out to compute a class would
          be a second source of truth for the same fact. */}
      <ChevronDown
        className={cn(
          "size-5 shrink-0 text-muted-foreground transition-transform group-data-[panel-open]:rotate-180",
          trailing ? "ml-3" : "ml-auto",
        )}
        aria-hidden
      />
    </Collapsible.Trigger>

    {/* The reveal: grid 0fr→1fr so the body slides open with no fixed height (styles.css).
        ⚠ `.reveal` keys off `data-open`, which Base UI emits VALUELESS when open — styles.css
        matches `[data-open=""]` alongside the `[data-open="true"]` that connection-block's
        React boolean produces. It deliberately does NOT match on attribute presence alone,
        because React renders `data-open={false}` as the STRING "false". */}
    <Collapsible.Panel hiddenUntilFound className="reveal">
      <div className="reveal-inner">
        <div className="border-border border-t p-5">{children}</div>
      </div>
    </Collapsible.Panel>
  </Collapsible.Root>
);

export { CollapsibleSection };
