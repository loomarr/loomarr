import { useRender } from "@base-ui/react/use-render";
import { cn } from "@/lib/utils";
import type { HighlightRowProps } from "./highlight-row.type";

// HighlightRow — one of Tonight's highlights (#1659 web mock): time, channel number, the show
// and why it is worth watching, on a fixed 80 / 36 / fill / auto grid so the columns line up
// down the list.
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
        "grid w-full cursor-pointer grid-cols-[80px_36px_minmax(0,1fr)_auto] items-center gap-3 border-none bg-transparent px-4 py-3 text-left text-foreground hover:bg-static-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
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
          <span className="whitespace-nowrap text-static-400 text-xs">{reason}</span>
        </>
      ),
    },
  });
  return <li className="border-static-700 border-b last:border-b-0">{control}</li>;
};

export { HighlightRow };
