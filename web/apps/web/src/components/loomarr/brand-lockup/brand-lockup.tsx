import { cn } from "@/lib";
import { ColorBars } from "../color-bars";
import type { BrandLockupProps } from "./brand-lockup.type";

// BrandLockup — the LOOMARR mark: the test-card strip over/beside the wordmark (§1). Two
// forms: `hero` (centered, big, tagline) for the login + wizard, and `compact`
// (horizontal, small) for the app sidebar. The wordmark is Geist, all-caps, wide-tracked;
// the tagline is mono, the same "nostalgia lives in microcopy" register as the rest (§1).
const BrandLockup = ({ variant = "hero", tagline = true, className }: BrandLockupProps) => {
  if (variant === "compact") {
    return (
      <div className={cn("flex items-center gap-2", className)}>
        <ColorBars size="sm" />
        <span className="font-semibold text-sm tracking-[0.12em]">LOOMARR</span>
      </div>
    );
  }
  return (
    <div className={cn("flex flex-col items-center gap-3", className)}>
      <ColorBars size="lg" />
      <span className="font-bold text-3xl tracking-[0.08em]">LOOMARR</span>
      {tagline && <span className="font-mono text-muted-foreground text-sm">always something on</span>}
    </div>
  );
};

export { BrandLockup };
