import type { PlayoutBackend } from "../steps";

interface LibraryStepProps {
  // Who plays the channels; the Sonarr and Radarr blocks read it as Connections does.
  backend?: PlayoutBackend;
}

export type { LibraryStepProps };
