type OnAirState = "off" | "live" | "reconciling";

interface OnAirIndicatorProps {
  state: OnAirState;
  showLabel?: boolean;
}

export type { OnAirIndicatorProps, OnAirState };
