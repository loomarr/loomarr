import { cn } from "@/lib";
import type { NowNextStripProps } from "./now-next-strip.type";

// NowNextStrip — the "Now: X · Next: Y" line for a channel (§3). Times + titles
// are mono (machine data, §2.2). A flex/pod gap reads as "Filler" rather than dead
// air; an unprogrammed channel reads "Nothing scheduled".
const NowNextStrip = ({ now, next, gap, className }: NowNextStripProps) => {
  if (!now && !gap) {
    return <p className={cn("text-sm text-static-400", className)}>Nothing scheduled</p>;
  }
  return (
    <p className={cn("truncate text-sm", className)}>
      <span className="text-static-400">Now </span>
      <span className="font-mono text-foreground">{gap ? "Filler break" : now?.title}</span>
      {now?.until && <span className="ml-1 font-mono text-static-400 text-xs">·til {now.until}</span>}
      {next && (
        <>
          <span className="mx-1.5 text-static-400">·</span>
          <span className="text-static-400">Next </span>
          <span className="font-mono text-static-100">{next.title}</span>
        </>
      )}
    </p>
  );
};

export { NowNextStrip };
