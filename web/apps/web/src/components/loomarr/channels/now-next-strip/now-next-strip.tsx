import { cn } from "@/lib/utils";
import type { NowNextStripProps } from "./now-next-strip.type";

// NowNextStrip — the "Now: X · Next: Y" line for a channel (§3). Times + titles
// are mono (machine data, §2.2). A flex/pod gap reads as "Filler" rather than dead
// air; an unprogrammed channel reads "Nothing scheduled".
const NowNextStrip = ({ now, next, gap, className }: NowNextStripProps) => {
  if (!now && !gap) {
    return <p className={cn("text-static-400 text-sm", className)}>Nothing scheduled</p>;
  }
  return (
    <p className={cn("truncate text-sm", className)}>
      <span className="text-static-400">Now </span>
      <span className="text-foreground font-mono">{gap ? "Filler break" : now?.title}</span>
      {now?.until && <span className="text-static-400 ml-1 font-mono text-xs">·til {now.until}</span>}
      {next && (
        <>
          <span className="text-static-400 mx-1.5">·</span>
          <span className="text-static-400">Next </span>
          <span className="text-static-100 font-mono">{next.title}</span>
        </>
      )}
    </p>
  );
};

export { NowNextStrip };
