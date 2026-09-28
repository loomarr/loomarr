import { useRender } from "@base-ui/react/use-render";
import { cn } from "@/lib/utils";
import type { HighlightRowProps } from "./highlight-row.type";

// HighlightRow — one of Tonight's highlights (#1659 web mock): time, channel number, the show
// and why it is worth watching, on a fixed 80 / 36 / fill / auto grid so the columns line up
// down the list. At phone width the reason drops under the show (#1785): on one line it took the
// show's whole column. The time column stays fixed there too, or each row's time sets its own width.
//
// Lives inside a `ListGroup`: the `li` carries the hairline, and the whole row is ONE control
// (the channel's Watch) with nothing interactive inside it.
const HighlightRow = ({
  time,
  channelNumber,
  title,
  detail,
  reason,
  render,
  onClick,
  className,
}: HighlightRowProps) => {
  const control = useRender({
    defaultTagName: "button",
    render,
    props: {
      onClick,
      className: cn(
        "grid w-full cursor-pointer grid-cols-[80px_36px_minmax(0,1fr)] items-center gap-x-3 gap-y-0.5 border-none bg-transparent px-4 py-3 text-left text-foreground hover:bg-static-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset sm:grid-cols-[80px_36px_minmax(0,1fr)_auto] sm:gap-3",
        className,
      ),
      children: (
        <>
          <span className="font-mono text-[13px]">{time}</span>
          <span className="font-mono font-semibold text-[13px] text-signal">{channelNumber}</span>
          <span className="min-w-0 truncate font-medium text-sm">
            {title}
            {detail != null && <span className="font-normal text-static-400">{detail}</span>}
          </span>
          <span className="col-start-3 text-static-400 text-xs sm:col-start-auto sm:whitespace-nowrap">
            {reason}
          </span>
        </>
      ),
    },
  });
  return <li className="border-static-700 border-b last:border-b-0">{control}</li>;
};

export { HighlightRow };
