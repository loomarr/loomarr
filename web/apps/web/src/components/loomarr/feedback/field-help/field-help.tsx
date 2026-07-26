import { Info } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui";
import { cn } from "@/lib";
import type { FieldHelpProps } from "./field-help.type";

// FieldHelp — a small (i) icon that shows a field's guidance on hover/focus, instead of a
// permanent muted paragraph under every control. This is what lets the settings forms breathe:
// the help is there when you want it, out of the way when you don't. Uses the app-wide Tooltip
// (TooltipProvider is mounted at the root, __root.tsx) so no local provider is needed.
//
// Accessible: the trigger is a real button labelled "About <field>", reachable by keyboard;
// the tooltip content is the help text. A screen-reader user tabs to it and hears the same
// guidance a sighted user gets on hover.
const FieldHelp = ({ children, label, className }: FieldHelpProps) => (
  <Tooltip>
    <TooltipTrigger asChild>
      <button
        type="button"
        aria-label={`About ${label}`}
        className={cn(
          "inline-flex size-4 shrink-0 cursor-help items-center justify-center rounded-full text-static-400 transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          className,
        )}
      >
        <Info className="size-3.5" aria-hidden />
      </button>
    </TooltipTrigger>
    <TooltipContent>{children}</TooltipContent>
  </Tooltip>
);

export { FieldHelp };
