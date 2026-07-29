import type { SystemVersionOutputBody } from "@loomarr/api";

interface AboutPanelProps {
  /** What the binary knows about itself, from GET /v1/system/version. */
  version: SystemVersionOutputBody;
  /**
   * Active database backend ("sqlite" | "postgres"), when it is known. Optional because
   * it comes from a different endpoint than the version, and About must render whether or
   * not that one resolved — a missing row is better than a blocked page.
   */
  backend?: string;
  className?: string;
}

export type { AboutPanelProps };
