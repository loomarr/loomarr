/** The palette roles a bar may take. §2.1 assigns `tune` to work in progress. */
type ProgressTone = "tune" | "signal" | "lock" | "onair";

interface ProgressProps {
  /**
   * How far along, 0-100.
   *
   * ⚠ **Omit it for INDETERMINATE** — that is the ARIA contract, not a convenience: a bar with
   * no `aria-valuenow` announces "busy", while `aria-valuenow={0}` announces "0 percent". They
   * are different claims, and only one of them is true of a task that cannot measure itself.
   * Never pass 0 to mean "no measurement yet".
   */
  value?: number;
  /** Required accessible name. A bar with no name is a rectangle to a screen reader. */
  label: string;
  tone?: ProgressTone;
  className?: string;
}

export type { ProgressProps, ProgressTone };
