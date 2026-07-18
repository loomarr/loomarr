interface BootstrapStepProps {
  // Called once the owning admin exists AND is signed in (bootstrap issues no session,
  // so the step logs in before reporting done).
  onDone: () => void;
}

export type { BootstrapStepProps };
