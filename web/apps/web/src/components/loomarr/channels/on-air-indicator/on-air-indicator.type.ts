type OnAirState = "off" | "live" | "reconciling";

interface OnAirIndicatorProps {
  state: OnAirState;
  showLabel?: boolean;
  className?: string;
}

export type { OnAirIndicatorProps, OnAirState };
