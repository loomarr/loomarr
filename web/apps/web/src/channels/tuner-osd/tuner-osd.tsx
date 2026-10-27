import { cn } from "@/lib/utils";
import type { TunerOSDProps } from "./tuner-osd.type";

// TunerOSD is the synchronous acknowledgement of a tune request (§9.1 V57). It names the target
// from the already-loaded surf catalog while transport catches up; it does not own media state.
const TunerOSD = ({ number, name, currentTitle, className }: TunerOSDProps) => (
  <div
    role="status"
    aria-live="polite"
    className={cn(
      "border-static-600/70 pointer-events-none max-w-[70%] rounded-md border bg-black/75 px-3 py-2 shadow-lg backdrop-blur-sm",
      className,
    )}
  >
    <p className="text-signal font-mono text-xs tracking-wide">CH {number}</p>
    <p className="text-static-50 truncate text-sm font-semibold">{name}</p>
    {currentTitle && <p className="text-static-300 truncate text-xs">{currentTitle}</p>}
    <p className="text-static-400 mt-1 font-mono text-[0.65rem] tracking-wider uppercase">Tuning…</p>
  </div>
);

export { TunerOSD };
