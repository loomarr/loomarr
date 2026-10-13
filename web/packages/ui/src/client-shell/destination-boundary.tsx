import { Component, type ReactNode } from "react";

type DestinationBoundaryProps = {
  children: ReactNode;
  /** Drawn in place of a destination that threw while rendering; `retry` renders it again. */
  fallback: (retry: () => void) => ReactNode;
};

type DestinationBoundaryState = { failed: boolean };

// One tab's render error stays in that tab. Without a boundary React unmounts the whole tree, so a
// screen that throws (Requests did, on a `null` the server sent where its contract promised an array)
// takes the tab bar with it and the app is a blank page until it is killed. The shell keys this by
// destination, so changing tabs also clears a failure.
class DestinationBoundary extends Component<DestinationBoundaryProps, DestinationBoundaryState> {
  override state: DestinationBoundaryState = { failed: false };

  static getDerivedStateFromError(): DestinationBoundaryState {
    return { failed: true };
  }

  private readonly retry = () => this.setState({ failed: false });

  override render() {
    return this.state.failed ? this.props.fallback(this.retry) : this.props.children;
  }
}

export { DestinationBoundary };
