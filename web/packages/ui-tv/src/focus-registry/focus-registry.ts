import type {
  FocusableTargetHandle,
  FocusTargetRegistry,
  GuideFocusTarget,
  SurfSelection,
} from "@loomarr/ui";

class TvFocusRegistry<TTarget> implements FocusTargetRegistry<TTarget> {
  private readonly handles = new Map<string, FocusableTargetHandle>();
  private pendingKey?: string;
  private focusedTarget?: TTarget;

  constructor(private readonly keyFor: (target: TTarget) => string) {}

  /** Where focus last landed (or was sent): the cell a Guide time edge was reached from. */
  current = (): TTarget | undefined => this.focusedTarget;

  focused = (target: TTarget) => {
    this.focusedTarget = target;
  };

  register = (target: TTarget, handle: FocusableTargetHandle | null) => {
    const key = this.keyFor(target);
    if (!handle) {
      this.handles.delete(key);
      return;
    }
    this.handles.set(key, handle);
    if (this.pendingKey === key) {
      this.pendingKey = undefined;
      this.focusedTarget = target;
      handle.focus?.();
    }
  };

  request = (target: TTarget) => {
    const key = this.keyFor(target);
    const handle = this.handles.get(key);
    if (handle) {
      this.pendingKey = undefined;
      // A requested focus is certain; the native onFocus that confirms it can lag.
      this.focusedTarget = target;
      handle.focus?.();
      return;
    }
    this.pendingKey = key;
  };
}

const guideFocusTargetKey = (target: GuideFocusTarget): string =>
  target.kind === "filter"
    ? `filter:${target.filter}`
    : `airing:${target.selection.channelId}:${target.selection.scheduleBlockId}`;

const surfFocusTargetKey = (target: SurfSelection): string => `${target.group}:${target.channelId}`;

const createTvGuideFocusRegistry = () => new TvFocusRegistry<GuideFocusTarget>(guideFocusTargetKey);
const createTvSurfFocusRegistry = () => new TvFocusRegistry<SurfSelection>(surfFocusTargetKey);

export { createTvGuideFocusRegistry, createTvSurfFocusRegistry, TvFocusRegistry };
