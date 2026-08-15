import { cn } from "@/lib/utils";
import type { TunerOSDProps } from "./tuner-osd.type";

// TunerOSD is the synchronous acknowledgement of a tune request (§9.1 V57). It names the target
// from the already-loaded surf catalog while transport catches up; it does not own media state.
const TunerOSD = ({ number, name, currentTitle, className }: TunerOSDProps) => (
  <div
    role="status"
    aria-live="polite"
    className={cn(
      "pointer-events-none max-w-[70%] rounded-md border border-static-600/70 bg-black/75 px-3 py-2 shadow-lg backdrop-blur-sm",
      className,
    )}
  >
    <p className="font-mono text-signal text-xs tracking-wide">CH {number}</p>
    <p className="truncate font-semibold text-sm text-static-50">{name}</p>
    {currentTitle && <p className="truncate text-static-300 text-xs">{currentTitle}</p>}
    <p className="mt-1 font-mono text-[0.65rem] text-static-400 uppercase tracking-wider">Tuning…</p>
  </div>
);

export { TunerOSD };
