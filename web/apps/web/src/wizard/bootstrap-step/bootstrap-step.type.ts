interface BootstrapStepProps {
  // Called once the owning admin exists AND is signed in (bootstrap issues no session,
  // so the step logs in before reporting done).
  onDone: () => void;
  // The signed-in owner, when there is one. Passed DOWN from the wizard route rather than
  // read from useAuth() here: the route already resolved identity to decide which step to
  // show, so a second subscription to the same query would be both redundant and a second
  // opinion about the same fact. (It was also observably wrong — mounting another observer
  // of the errored `me` query kept the route's own `isLoading` true, so the wizard sat on
  // its spinner forever and the unauthenticated bootstrap form never rendered.)
  ownerName?: string;
}

export type { BootstrapStepProps };
