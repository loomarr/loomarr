import { ChevronDown } from "lucide-react";
import { useId, useState } from "react";
import { cn } from "@/lib";
import type { CollapsibleSectionProps } from "./collapsible-section.type";

// CollapsibleSection — the app's accordion section (§12/§13): a bordered card whose body
// slides open/closed from a clickable header. Extracted from the wizard's checklist blocks
// (the proven pattern) so the channel-detail workbench can present its editors calm — each
// section collapsed until you open the one you're tuning, instead of a wall of expanded forms.
//
// The open/close uses the `.reveal` grid-rows 0fr→1fr trick (styles.css) — height-agnostic
// (no measuring) and frozen under reduced-motion. The header is a real <button aria-expanded>
// so it's keyboard + screen-reader operable; the body is genuinely hidden (overflow-clipped)
// while closed. Uncontrolled by default (owns `open`); pass open/onOpenChange to control it.
const CollapsibleSection = ({
  title,
  description,
  icon,
  trailing,
  defaultOpen = false,
  open: controlledOpen,
  onOpenChange,
  children,
  className,
}: CollapsibleSectionProps) => {
  const bodyId = useId();
  const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen);
  const isControlled = controlledOpen !== undefined;
  const open = isControlled ? controlledOpen : uncontrolledOpen;

  const toggle = () => {
    const next = !open;
    if (!isControlled) setUncontrolledOpen(next);
    onOpenChange?.(next);
  };

  return (
    <section className={cn("overflow-hidden rounded-lg border border-border", className)}>
      <button
        type="button"
        onClick={toggle}
        aria-expanded={open}
        aria-controls={bodyId}
        className="flex w-full cursor-pointer items-center gap-3 px-5 py-4 text-left transition-colors hover:bg-static-800"
      >
        {icon && <span className="flex shrink-0 items-center text-muted-foreground">{icon}</span>}
        <span className="min-w-0">
          <span className="block font-semibold text-lg leading-tight">{title}</span>
          {description && <span className="mt-0.5 block text-muted-foreground text-sm">{description}</span>}
        </span>
        {trailing && <span className="ml-auto shrink-0">{trailing}</span>}
        <ChevronDown
          className={cn(
            "size-5 shrink-0 text-muted-foreground transition-transform",
            trailing ? "ml-3" : "ml-auto",
            open && "rotate-180",
          )}
          aria-hidden
        />
      </button>

      {/* The reveal: grid 0fr→1fr so the body slides open with no fixed height (styles.css). */}
      <div id={bodyId} className="reveal" data-open={open}>
        <div className="reveal-inner">
          <div className="border-border border-t p-5">{children}</div>
        </div>
      </div>
    </section>
  );
};

export { CollapsibleSection };
