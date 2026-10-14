import { Component, type ReactNode } from "react";

type DestinationBoundaryProps = {
  children: ReactNode;
  /** Drawn in place of a destination that threw while rendering; `retry` renders it again. */
  fallback: (retry: () => void) => ReactNode;
  /** The destination shown; changing it clears a failure without remounting what stays (the tab bar). */
  resetKey: string;
};

type DestinationBoundaryState = { failed: boolean; resetKey: string };

// One tab's render error stays in that tab. Without a boundary React unmounts the whole tree, so a
// screen that throws (Requests did, on a `null` the server sent where its contract promised an array)
// takes the tab bar with it and the app is a blank page until it is killed. Changing tabs also clears
// a failure.
class DestinationBoundary extends Component<DestinationBoundaryProps, DestinationBoundaryState> {
  override state: DestinationBoundaryState = { failed: false, resetKey: this.props.resetKey };

  static getDerivedStateFromError(): Partial<DestinationBoundaryState> {
    return { failed: true };
  }

  // In the same render as the new destination, so its first frame is never the old failure.
  static getDerivedStateFromProps(
    props: DestinationBoundaryProps,
    state: DestinationBoundaryState,
  ): DestinationBoundaryState | null {
    return props.resetKey === state.resetKey ? null : { failed: false, resetKey: props.resetKey };
  }

  private readonly retry = () => this.setState({ failed: false });

  override render() {
    return this.state.failed ? this.props.fallback(this.retry) : this.props.children;
  }
}

export { DestinationBoundary };
