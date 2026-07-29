import type { PlayoutBackend } from "../steps";

interface PlayoutStepProps {
  // The currently-saved backend, resolved by the route from the settings registry. Always
  // has a value (the registry defaults to internal), so this step CONFIRMS a choice rather
  // than collecting one from nothing.
  value: PlayoutBackend;
  // Env-pinned installs cannot change this here (config-design §3): the field is locked and
  // the step says which variable owns it, rather than offering a control that silently fails.
  pinnedBy?: string;
}

export type { PlayoutStepProps };
