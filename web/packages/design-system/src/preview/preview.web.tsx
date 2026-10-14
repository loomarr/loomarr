/// <reference lib="dom" />
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import type { PreviewAnchorProps, PreviewGroupProps } from "./preview.type";

const INTENT_MS = 250;
const DEPARTURE_MS = 180;
const INSET = 8;
type Target = { element: HTMLElement; content: ReactNode };
type Controller = {
  enter: (target: Target) => void;
  leave: (element: HTMLElement) => void;
};
const Context = createContext<Controller | undefined>(undefined);

// One preview per browsing surface. It never moves focus or changes the initiating action.
const PreviewGroup = ({ children, resetKey }: PreviewGroupProps) => {
  const id = useId();
  const [active, setActive] = useState<Target>();
  const [position, setPosition] = useState({ left: INSET, top: INSET });
  const pending = useRef<Target | undefined>(undefined);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const panel = useRef<HTMLDivElement>(null);
  const clearTimer = useCallback(() => {
    clearTimeout(timer.current);
    timer.current = undefined;
  }, []);
  const dismiss = useCallback(() => {
    clearTimer();
    pending.current = undefined;
    setActive(undefined);
  }, [clearTimer]);
  const enter = useCallback(
    (target: Target) => {
      clearTimer();
      pending.current = target;
      timer.current = setTimeout(() => {
        if (target.element.isConnected) setActive(target);
      }, INTENT_MS);
    },
    [clearTimer],
  );
  const leave = useCallback(
    (element: HTMLElement) => {
      if (pending.current?.element !== element) return;
      clearTimer();
      timer.current = setTimeout(() => {
        if (
          element.contains(document.activeElement) ||
          element.matches(":hover") ||
          panel.current?.contains(document.activeElement) ||
          panel.current?.matches(":hover")
        )
          return;
        dismiss();
      }, DEPARTURE_MS);
    },
    [clearTimer, dismiss],
  );
  const controller = useMemo(() => ({ enter, leave }), [enter, leave]);

  useEffect(() => {
    void resetKey;
    dismiss();
  }, [resetKey, dismiss]);
  useEffect(() => clearTimer, [clearTimer]);
  useEffect(() => {
    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && pending.current) {
        dismiss();
        event.preventDefault();
      }
    };
    document.addEventListener("keydown", onEscape);
    return () => document.removeEventListener("keydown", onEscape);
  }, [dismiss]);

  useLayoutEffect(() => {
    if (!active || !panel.current) return;
    const anchor = active.element;
    const previousDescription = anchor.getAttribute("aria-describedby");
    anchor.setAttribute("aria-describedby", [previousDescription, id].filter(Boolean).join(" "));
    const place = () => {
      const box = anchor.getBoundingClientRect();
      if (
        !anchor.isConnected ||
        box.width <= 0 ||
        box.height <= 0 ||
        box.bottom <= 0 ||
        box.top >= window.innerHeight
      ) {
        dismiss();
        return;
      }
      const height = panel.current?.getBoundingClientRect().height ?? 0;
      const width = Math.min(360, window.innerWidth - INSET * 2);
      const top =
        box.bottom + height + INSET <= window.innerHeight ? box.bottom : Math.max(INSET, box.top - height);
      setPosition({ left: Math.max(INSET, Math.min(box.left, window.innerWidth - width - INSET)), top });
    };
    place();
    const observer = new ResizeObserver(place);
    observer.observe(panel.current);
    // Scrolling invalidates a virtualised anchor, even when its row is recycled.
    const onScroll = (event: Event) => {
      if (event.target instanceof Node && panel.current?.contains(event.target)) return;
      dismiss();
    };
    window.addEventListener("resize", place);
    window.addEventListener("scroll", onScroll, true);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", onScroll, true);
      if (previousDescription === null) anchor.removeAttribute("aria-describedby");
      else anchor.setAttribute("aria-describedby", previousDescription);
    };
  }, [active, dismiss, id]);

  return (
    <Context.Provider value={controller}>
      {children}
      {active &&
        createPortal(
          <div
            id={id}
            role="tooltip"
            ref={panel}
            onPointerEnter={clearTimer}
            onPointerLeave={() => leave(active.element)}
            onFocusCapture={clearTimer}
            onBlurCapture={() => leave(active.element)}
            style={{
              position: "fixed",
              ...position,
              width: "min(360px, calc(100vw - 16px))",
              maxHeight: "calc(100vh - 16px)",
              overflow: "auto",
              zIndex: 100,
            }}
          >
            {active.content}
          </div>,
          document.body,
        )}
    </Context.Provider>
  );
};

const PreviewAnchor = ({ children, content }: PreviewAnchorProps) => {
  const controller = useContext(Context);
  const wrapper = useRef<HTMLDivElement>(null);
  const touch = useRef(false);
  const element = () => wrapper.current?.firstElementChild as HTMLElement | undefined;
  if (!content || !controller) return <>{children}</>;
  return (
    <div
      ref={wrapper}
      style={{ display: "contents" }}
      onPointerDownCapture={(event) => {
        touch.current = event.pointerType === "touch";
      }}
      onPointerEnter={(event) => {
        if (event.pointerType === "touch") return;
        const anchor = element();
        if (anchor) controller.enter({ element: anchor, content });
      }}
      onPointerLeave={() => {
        const anchor = element();
        if (anchor) controller.leave(anchor);
      }}
      onFocusCapture={() => {
        if (touch.current) return;
        const anchor = element();
        if (anchor) controller.enter({ element: anchor, content });
      }}
      onBlurCapture={() => {
        touch.current = false;
        const anchor = element();
        if (anchor) controller.leave(anchor);
      }}
    >
      {children}
    </div>
  );
};

export { PreviewAnchor, PreviewGroup };
