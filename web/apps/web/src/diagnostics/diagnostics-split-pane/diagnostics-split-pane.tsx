import { GripVertical, PanelRightClose, PanelRightOpen } from "lucide-react";
import {
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
  useEffect,
  useRef,
  useState,
} from "react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

const MIN_DETAIL_WIDTH = 288;
const MAX_DETAIL_WIDTH = 640;
const DEFAULT_DETAIL_WIDTH = 352;

const clampWidth = (width: number) => Math.min(MAX_DETAIL_WIDTH, Math.max(MIN_DETAIL_WIDTH, width));

const storedWidth = (storageKey: string) => {
  try {
    const value = Number(sessionStorage.getItem(`loomarr:${storageKey}:width`));
    return Number.isFinite(value) && value > 0 ? clampWidth(value) : DEFAULT_DETAIL_WIDTH;
  } catch {
    return DEFAULT_DETAIL_WIDTH;
  }
};

const DiagnosticsSplitPane = ({
  storageKey,
  primary,
  secondary,
  revealKey,
  breakpoint = "lg",
  secondaryOnMobile = false,
  className,
}: {
  storageKey: string;
  primary: ReactNode;
  secondary: ReactNode;
  revealKey?: string;
  breakpoint?: "lg" | "xl";
  secondaryOnMobile?: boolean;
  className?: string;
}) => {
  const [width, setWidth] = useState(() => storedWidth(storageKey));
  const [collapsed, setCollapsed] = useState(false);
  const drag = useRef<{ x: number; width: number } | undefined>(undefined);
  const desktop = breakpoint === "lg" ? "lg" : "xl";
  const layout =
    breakpoint === "lg"
      ? collapsed
        ? "lg:grid-cols-[minmax(0,1fr)_2.5rem] lg:gap-0"
        : "lg:grid-cols-[minmax(0,1fr)_2.25rem_var(--diagnostics-detail-width)] lg:gap-0"
      : collapsed
        ? "xl:grid-cols-[minmax(0,1fr)_2.5rem] xl:gap-0"
        : "xl:grid-cols-[minmax(0,1fr)_2.25rem_var(--diagnostics-detail-width)] xl:gap-0";
  const desktopFlex = desktop === "lg" ? "hidden lg:flex" : "hidden xl:flex";
  const secondaryVisibility = secondaryOnMobile
    ? collapsed
      ? breakpoint === "lg"
        ? "lg:hidden"
        : "xl:hidden"
      : ""
    : collapsed
      ? "hidden"
      : breakpoint === "lg"
        ? "hidden lg:block"
        : "hidden xl:block";

  useEffect(() => {
    if (revealKey) setCollapsed(false);
  }, [revealKey]);

  const updateWidth = (next: number) => {
    const bounded = clampWidth(next);
    setWidth(bounded);
    try {
      sessionStorage.setItem(`loomarr:${storageKey}:width`, String(bounded));
    } catch {
      // Storage can be unavailable in privacy modes; resizing still works for this render.
    }
  };

  const resizeWithKeyboard = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key === "ArrowLeft") updateWidth(width + 16);
    else if (event.key === "ArrowRight") updateWidth(width - 16);
    else if (event.key === "Home") updateWidth(MIN_DETAIL_WIDTH);
    else if (event.key === "End") updateWidth(MAX_DETAIL_WIDTH);
    else return;
    event.preventDefault();
  };

  const beginDrag = (event: PointerEvent<HTMLElement>) => {
    drag.current = { x: event.clientX, width };
    event.currentTarget.setPointerCapture(event.pointerId);
  };

  const moveDrag = (event: PointerEvent<HTMLElement>) => {
    if (!drag.current) return;
    updateWidth(drag.current.width + drag.current.x - event.clientX);
  };

  return (
    <div
      className={cn("grid gap-3", layout, className)}
      style={{ "--diagnostics-detail-width": `${width}px` } as CSSProperties}
    >
      <div className="min-w-0 overflow-x-hidden">{primary}</div>
      {collapsed ? (
        <div className={cn(desktopFlex, "items-start justify-center pt-1")}>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Show details"
                  onClick={() => setCollapsed(false)}
                >
                  <PanelRightOpen aria-hidden />
                </Button>
              }
            />
            <TooltipContent>Show details</TooltipContent>
          </Tooltip>
        </div>
      ) : (
        <div className={cn(desktopFlex, "min-h-0 flex-col items-center")}>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Hide details"
                  onClick={() => setCollapsed(true)}
                >
                  <PanelRightClose aria-hidden />
                </Button>
              }
            />
            <TooltipContent>Hide details</TooltipContent>
          </Tooltip>
          <div className="group relative flex min-h-24 flex-1 items-center justify-center self-stretch">
            <hr
              aria-label="Resize details"
              aria-orientation="vertical"
              aria-valuemin={MIN_DETAIL_WIDTH}
              aria-valuemax={MAX_DETAIL_WIDTH}
              aria-valuenow={width}
              tabIndex={0}
              className="absolute inset-0 h-full w-full cursor-col-resize touch-none rounded-sm border-0 outline-none hover:bg-signal-tint-8 focus-visible:ring-2 focus-visible:ring-ring"
              onKeyDown={resizeWithKeyboard}
              onPointerDown={beginDrag}
              onPointerMove={moveDrag}
              onPointerUp={() => {
                drag.current = undefined;
              }}
              onPointerCancel={() => {
                drag.current = undefined;
              }}
            />
            <GripVertical
              aria-hidden
              className="pointer-events-none relative size-4 text-muted-foreground group-hover:text-signal"
            />
          </div>
        </div>
      )}
      <div
        data-testid="diagnostics-secondary-pane"
        className={cn("min-w-0 overflow-x-hidden", secondaryVisibility)}
      >
        {secondary}
      </div>
    </div>
  );
};

export { DiagnosticsSplitPane };
