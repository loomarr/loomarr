type ProvisioningState = "wanted" | "requested" | "downloading" | "available" | "unavailable" | "drift";

interface StateBadgeProps {
  state: ProvisioningState;
  className?: string;
}

export type { ProvisioningState, StateBadgeProps };
