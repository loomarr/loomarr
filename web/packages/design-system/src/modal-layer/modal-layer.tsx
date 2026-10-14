import type { ReactNode } from "react";
import { createContext, useContext, useEffect, useSyncExternalStore } from "react";

/**
 * Which modal layers are open, so a control behind one can drop its focus look.
 *
 * A native `Modal` moves focus into its own window without blurring the control that opened it,
 * so that control's `focused` state stays true and its amber border shows through the scrim
 * (seen on TV, #1909). The web adapter blurs the opener, so this is harmless there. Each layer
 * provides its depth to its own tree and registers it here while visible; a control is behind a
 * modal when an open layer is deeper than the layer it renders in.
 */
const openDepths: number[] = [];
const listeners = new Set<() => void>();

const deepestOpen = () => (openDepths.length ? Math.max(...openDepths) : 0);

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

const notify = () => {
  for (const listener of listeners) listener();
};

const ModalDepthContext = createContext(0);

type ModalLayerProps = {
  children: ReactNode;
  visible: boolean;
};

/** Wrap a modal's content. Controls inside it keep their focus look while it is open. */
const ModalLayer = ({ children, visible }: ModalLayerProps) => {
  const depth = useContext(ModalDepthContext) + 1;
  useEffect(() => {
    if (!visible) return;
    openDepths.push(depth);
    notify();
    return () => {
      openDepths.splice(openDepths.indexOf(depth), 1);
      notify();
    };
  }, [depth, visible]);
  return <ModalDepthContext.Provider value={depth}>{children}</ModalDepthContext.Provider>;
};

/** True while a modal layer above this component's own layer is open. */
const useBehindModal = () => {
  const depth = useContext(ModalDepthContext);
  const deepest = useSyncExternalStore(subscribe, deepestOpen, deepestOpen);
  return deepest > depth;
};

export type { ModalLayerProps };
export { ModalLayer, useBehindModal };
