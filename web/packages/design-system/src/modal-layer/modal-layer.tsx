import type { ReactNode } from "react";
import { createContext, useContext, useId, useLayoutEffect, useSyncExternalStore } from "react";

/**
 * Which modal layers are open, so a control behind one can drop its focus look.
 *
 * A native `Modal` moves focus into its own window without blurring the control that opened it,
 * so that control's `focused` state stays true and its amber border shows through the scrim
 * (seen on TV, #1909). The web adapter blurs the opener, so this is harmless there. Each layer
 * provides its own id to its tree and joins a stack in the order it opened. Only the topmost open
 * layer is in front: a control anywhere else, including inside a modal that another modal opened
 * over, is behind. Opening order, not tree nesting, decides, because sibling modals are common.
 */
let openLayers: readonly string[] = [];
const listeners = new Set<() => void>();

const topmost = () => openLayers.at(-1);

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

const setOpenLayers = (next: readonly string[]) => {
  openLayers = next;
  for (const listener of listeners) listener();
};

const ModalLayerContext = createContext<string | undefined>(undefined);

type ModalLayerProps = {
  children: ReactNode;
  visible: boolean;
};

/** Wrap a modal's content. Controls inside it keep their focus look while it is the top layer. */
const ModalLayer = ({ children, visible }: ModalLayerProps) => {
  const id = useId();
  // A layout effect, so the opener drops its focus look in the same frame the modal appears.
  useLayoutEffect(() => {
    if (!visible) return;
    setOpenLayers([...openLayers, id]);
    return () => setOpenLayers(openLayers.filter((layer) => layer !== id));
  }, [id, visible]);
  return <ModalLayerContext.Provider value={id}>{children}</ModalLayerContext.Provider>;
};

/** True while an open modal layer other than this component's own is on top. */
const useBehindModal = () => {
  const own = useContext(ModalLayerContext);
  const top = useSyncExternalStore(subscribe, topmost, topmost);
  return top !== undefined && top !== own;
};

export type { ModalLayerProps };
export { ModalLayer, useBehindModal };
