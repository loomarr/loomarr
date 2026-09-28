import { mergeProps } from "@base-ui/react/merge-props";
import { useRender } from "@base-ui/react/use-render";
import { cn } from "@/lib/utils";
import type { SectionHeaderActionProps, SectionHeaderProps } from "./section-header.type";

// SectionHeader — a Home section's title, its meta, and a trailing link (#1659 web mock: Watching
// now, Channel ideas, Tonight, New this week, On the way).
//
// Baseline-aligned so the 12 px meta sits on the title's baseline, not its centre. It wraps: at a
// narrow width the trailing link drops to its own line instead of pushing the title into a
// word-per-line column.
const SectionHeader = ({ title, meta, children, id, className }: SectionHeaderProps) => (
  <div className={cn("mb-3 flex flex-wrap items-baseline gap-x-3 gap-y-1", className)}>
    <h2 id={id} className="m-0 font-semibold text-lg">
      {title}
    </h2>
    {meta != null && <span className="text-static-400 text-xs">{meta}</span>}
    {children != null && <div className="ml-auto flex items-center gap-3 self-center">{children}</div>}
  </div>
);

// SectionHeaderAction — the header's quiet trailing link ("Full guide →", "All requests →").
// The arrow is decoration beside the words, so it is hidden from assistive tech.
const SectionHeaderAction = ({ className, render, children, ...props }: SectionHeaderActionProps) =>
  useRender({
    defaultTagName: "button",
    render,
    props: mergeProps<"button">(
      {
        className: cn(
          "cursor-pointer border-none bg-transparent p-0 text-[13px] text-static-400 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          className,
        ),
        children: (
          <>
            {children} <span aria-hidden="true">→</span>
          </>
        ),
      },
      props,
    ),
  });

export { SectionHeader, SectionHeaderAction };
