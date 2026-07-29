import type { SystemVersionOutputBody } from "@loomarr/api";

interface AboutPanelProps {
  /**
   * What the binary knows about itself, from GET /v1/system/version — including the Go
   * runtime, process start, database backend and applied schema version.
   *
   * Every row comes from here. There is deliberately no second source: the backend used to
   * be passed separately from the database endpoint, which made "which one is right?" a
   * question this panel could ask about its own content.
   */
  version: SystemVersionOutputBody;
  /**
   * Clock for the derived uptime, injectable so tests and stories are deterministic — an
   * uptime computed from the real clock would churn a visual baseline on every run.
   */
  now?: number;
  className?: string;
}

export type { AboutPanelProps };
