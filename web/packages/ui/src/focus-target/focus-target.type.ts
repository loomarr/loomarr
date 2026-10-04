interface FocusableTargetHandle {
  focus?: () => void;
}

interface FocusTargetRegistry<TTarget> {
  /** The platform's focus engine landed on this target (a D-pad move, a restore, a pointer). */
  focused?: (target: TTarget) => void;
  register: (target: TTarget, handle: FocusableTargetHandle | null) => void;
  request: (target: TTarget) => void;
}

export type { FocusableTargetHandle, FocusTargetRegistry };
