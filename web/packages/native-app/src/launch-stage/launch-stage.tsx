import { BrandLaunch } from "@loomarr/design-system";
import { type ComponentProps, type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { View } from "react-native";

interface LaunchGateOptions {
  /** Hides the native splash screen; called once, at the first layout. */
  hideNativeSplash: () => void;
  /** How long the launch identity stays up at least, so a fast start does not flash it. */
  minimumMs: number;
}

/**
 * Whether the launch identity is still showing: until its animation has finished and it has been up
 * for `minimumMs`, counted from the first render. The native splash hides once, at the first layout.
 */
const useLaunchGate = ({ hideNativeSplash, minimumMs }: LaunchGateOptions) => {
  const [animationFinished, setAnimationFinished] = useState(false);
  const [minimumElapsed, setMinimumElapsed] = useState(false);
  const startedAt = useRef(Date.now());
  const splashHidden = useRef(false);
  const onLayout = useCallback(() => {
    if (splashHidden.current) return;
    splashHidden.current = true;
    hideNativeSplash();
  }, [hideNativeSplash]);
  useEffect(() => {
    const remaining = Math.max(0, minimumMs - (Date.now() - startedAt.current));
    const timer = setTimeout(() => setMinimumElapsed(true), remaining);
    return () => clearTimeout(timer);
  }, [minimumMs]);
  const onAnimationFinished = useCallback(() => setAnimationFinished(true), []);
  return { launching: !(animationFinished && minimumElapsed), onAnimationFinished, onLayout };
};

interface LaunchStageProps extends LaunchGateOptions {
  children: ReactNode;
  density: NonNullable<ComponentProps<typeof BrandLaunch>["density"]>;
}

/**
 * Draws the Loomarr launch identity over the app. The app underneath mounts at once, so pairing
 * initializes while the identity plays instead of waiting for its animation.
 */
const LaunchStage = ({ children, density, hideNativeSplash, minimumMs }: LaunchStageProps) => {
  const { launching, onAnimationFinished, onLayout } = useLaunchGate({ hideNativeSplash, minimumMs });
  return (
    <View onLayout={onLayout} style={{ flex: 1 }}>
      {children}
      {launching ? (
        <View style={{ bottom: 0, left: 0, position: "absolute", right: 0, top: 0 }}>
          <BrandLaunch density={density} onFinished={onAnimationFinished} />
        </View>
      ) : null}
    </View>
  );
};

export type { LaunchGateOptions, LaunchStageProps };
export { LaunchStage, useLaunchGate };
