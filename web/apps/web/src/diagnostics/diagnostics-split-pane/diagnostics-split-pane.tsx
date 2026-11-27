import {
  Action,
  breakpoints,
  Grid,
  Icon,
  icons,
  Stack,
  Tooltip,
  useWindowWidth,
} from "@loomarr/design-system";
import {
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";

const MIN_DETAIL_WIDTH = 288;
const MAX_DETAIL_WIDTH = 640;
const DEFAULT_DETAIL_WIDTH = 352;
// The design system names no `xl`, so the wider split reads the window's width against this.
const XL_WIDTH = 1280;

// Whether the window is at least `width` wide. It reads the browser's min-width media query, which
// counts a classic scrollbar as a breakpoint does, and falls back to the window's width where there
// is no matchMedia (jsdom).
const useMinWidth = (width: number) => {
  const windowWidth = useWindowWidth();
  const query = (query: string) => globalThis.matchMedia?.(query);
  return useSyncExternalStore(
    (onChange) => {
      const list = query(`(min-width: ${width}px)`);
      list?.addEventListener("change", onChange);
      return () => list?.removeEventListener("change", onChange);
    },
    () => query(`(min-width: ${width}px)`)?.matches ?? windowWidth >= width,
    () => false,
  );
};

// The resize handle's hairline: it fills its cell, takes a signal wash while pointed at, and draws
// the focus ring only for the keyboard (the ring is state because inline style has no :focus-visible).
const handleStyle = (hovered: boolean, ringed: boolean): CSSProperties => ({
  background: hovered ? "var(--color-signal-tint-8)" : "transparent",
  border: 0,
  borderRadius: 2,
  cursor: "col-resize",
  height: "100%",
  inset: 0,
  margin: 0,
  outline: ringed ? "2px solid var(--actionFocus)" : "none",
  position: "absolute",
  touchAction: "none",
  width: "100%",
});

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
  flex,
  minHeight,
}: {
  storageKey: string;
  primary: ReactNode;
  secondary: ReactNode;
  revealKey?: string;
  breakpoint?: "lg" | "xl";
  secondaryOnMobile?: boolean;
  flex?: number;
  minHeight?: number;
}) => {
  const [width, setWidth] = useState(() => storedWidth(storageKey));
  const [collapsed, setCollapsed] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [ringed, setRinged] = useState(false);
  const drag = useRef<{ x: number; width: number } | undefined>(undefined);
  const desktop = useMinWidth(breakpoint === "lg" ? breakpoints.lg : XL_WIDTH);
  const columns = desktop
    ? collapsed
      ? (["minmax(0,1fr)", "2.5rem"] as const)
      : (["minmax(0,1fr)", "2.25rem", `${width}px`] as const)
    : 1;
  // Below the breakpoint the detail pane stacks under the list, or is left out when it is not meant
  // to appear on a small screen. At the breakpoint a collapsed pane gives its column to the rail.
  const secondaryHidden = secondaryOnMobile ? desktop && collapsed : collapsed || !desktop;

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
    <Grid
      columns={columns}
      gap={desktop ? "none" : "3"}
      minHeight={minHeight}
      // A percentage basis, not `flex`: in a column with no definite height it sizes to the content,
      // where `flex` gives a 0px basis and holds the pane at `minHeight`, clipping the list.
      {...(flex === undefined ? {} : { flexBasis: "0%", flexGrow: flex, flexShrink: 1 })}
    >
      <Stack minWidth={0} overflowX="hidden">
        {primary}
      </Stack>
      {collapsed ? (
        <Stack align="start" direction="row" hidden={!desktop} justify="center" paddingTop={4}>
          <Tooltip content="Show details">
            <Action icon="paneOpen" layout="icon" onPress={() => setCollapsed(false)} variant="ghost">
              Show details
            </Action>
          </Tooltip>
        </Stack>
      ) : (
        <Stack align="center" hidden={!desktop} minHeight={0}>
          <Tooltip content="Hide details">
            <Action icon="paneClose" layout="icon" onPress={() => setCollapsed(true)} variant="ghost">
              Hide details
            </Action>
          </Tooltip>
          <Stack
            align="center"
            alignSelf="stretch"
            direction="row"
            flexBasis="0%"
            flexGrow={1}
            flexShrink={1}
            justify="center"
            minHeight={96}
            onPointerEnter={() => setHovered(true)}
            onPointerLeave={() => setHovered(false)}
            position="relative"
          >
            <hr
              aria-label="Resize details"
              aria-orientation="vertical"
              aria-valuemin={MIN_DETAIL_WIDTH}
              aria-valuemax={MAX_DETAIL_WIDTH}
              aria-valuenow={width}
              tabIndex={0}
              style={handleStyle(hovered, ringed)}
              onBlur={() => setRinged(false)}
              onFocus={(event) => setRinged(event.currentTarget.matches(":focus-visible"))}
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
            <Stack direction="row" pointerEvents="none" position="relative" render="span">
              <Icon
                color={hovered ? "var(--color-signal)" : undefined}
                decorative
                glyph={icons.dragHandle}
                size="compact"
                tone="muted"
              />
            </Stack>
          </Stack>
        </Stack>
      )}
      <Stack
        data-testid="diagnostics-secondary-pane"
        hidden={secondaryHidden}
        minWidth={0}
        overflowX="hidden"
      >
        {secondary}
      </Stack>
    </Grid>
  );
};

export { DiagnosticsSplitPane };
