import type { ReactNode } from "react";

import type { TextTone } from "../text";
import type { TextRole } from "../tokens";

type DetailsProps = {
  /** What the summary opens onto. It is laid out by the caller: the Details adds no padding or border. */
  children: ReactNode;
  /** Start open and let the reader decide from there. Ignored when `open` is given. */
  defaultOpen?: boolean;
  /**
   * Keep the summary to the width of its words, so only the words open it. Left out, the summary is a
   * full-width line and a click anywhere along it opens.
   */
  fitSummary?: boolean;
  /**
   * The disclosure marker (the native triangle) before the summary. It is on by default, which is the
   * bare look: a marker and a line. `false` takes it away, for a summary that leads with its own glyph.
   */
  marker?: boolean;
  /** Called with the new state when the reader opens or closes it, or when `open` changes it. */
  onToggle?: (open: boolean) => void;
  /**
   * The open state, for a Details a page keeps open (a section with something to fix). It follows
   * the prop whenever the prop changes and the reader can still toggle in between.
   */
  open?: boolean;
  /**
   * The summary line. Given as a function it receives whether the Details is open, to swap its words
   * ("Show error" and "Hide error") without any style of the caller's own.
   */
  summary: ReactNode | ((open: boolean) => ReactNode);
  /** The summary's text role. Defaults to `compact`. */
  textRole?: TextRole;
  /** The summary's colour. Left out, the role's own. */
  tone?: TextTone;
};

export type { DetailsProps };
