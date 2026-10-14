import { semanticRadius } from "@loomarr/design-system";
import * as React from "react";
import { cn } from "@/lib/utils";

// shadcn/ui Card (new-york), restyled via tokens (§3, Layer 1). Elevation is
// borders-first in dark UIs (§2.3): surface fill + a static-700 hairline.
//
// `border-border`/`bg-card`/`text-card-foreground` stay literal Tailwind classes rather than
// moving to design-system tokens (#970 PR B checkpoint 2): several real call sites override them
// via conflicting Tailwind colour utilities (`border-caution/40 bg-caution/5` at filler-page.tsx,
// `border-signal/35` at filler-overview.tsx, `text-muted-foreground` at filler-manage.tsx), which
// only works because `cn`'s tailwind-merge can drop the base class in favour of the caller's.
// `rounded-lg` has no such caller — this one, alone, moves to inline style, sourced from
// design-system's own `$cardCompact` (12px) radius that the new `Card` primitive there also uses.
const Card = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, style, ...props }, ref) => (
    <div
      ref={ref}
      className={cn("border border-border bg-card text-card-foreground", className)}
      style={{ borderRadius: semanticRadius.cardCompact, ...style }}
      {...props}
    />
  ),
);
Card.displayName = "Card";

const CardHeader = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div ref={ref} className={cn("flex flex-col gap-1.5 p-4", className)} {...props} />
  ),
);
CardHeader.displayName = "CardHeader";

const CardTitle = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div ref={ref} className={cn("font-semibold leading-none tracking-tight", className)} {...props} />
  ),
);
CardTitle.displayName = "CardTitle";

const CardContent = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("p-4 pt-0", className)} {...props} />,
);
CardContent.displayName = "CardContent";

const CardFooter = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div ref={ref} className={cn("flex items-center p-4 pt-0", className)} {...props} />
  ),
);
CardFooter.displayName = "CardFooter";

export { Card, CardContent, CardFooter, CardHeader, CardTitle };
