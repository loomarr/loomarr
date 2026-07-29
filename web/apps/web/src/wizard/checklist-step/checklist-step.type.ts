import type { PlayoutBackend } from "../steps";

interface ChecklistStepProps {
  // The connection block currently revealed (its id === the check name), controlled by the
  // wizard route so the rail's sub-items and the content stay in sync. Undefined = all
  // collapsed.
  openId?: string;
  onToggle?: (id: string) => void;
  // Who plays the channels (§9.1). Decides whether the Tunarr block exists at all, and which
  // blocks count as required. Resolved by the route so the rail and the content agree.
  backend?: PlayoutBackend;
}

export type { ChecklistStepProps };
