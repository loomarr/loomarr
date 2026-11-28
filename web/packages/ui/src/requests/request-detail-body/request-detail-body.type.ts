import type { RequestEntry, TitleDTO } from "@loomarr/core/requests";
import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

type RequestDetailBodyProps = {
  entry: RequestEntry;
  titles: readonly TitleDTO[];
  /** The approval's date as the client words it, read after the approver ("on Oct 3", "25d ago"). */
  whenText: (iso: string) => string;
  /** `touch` on native; the comfortable default elsewhere. */
  density?: Density;
  /** Makes section titles real headings on the web. */
  headingLevel?: "h2";
  /** Buttons for the whole request, between why it failed and the decision. */
  actions?: ReactNode;
  /** Controls and errors for deciding, inside Decision. */
  decisionActions?: ReactNode;
  /** The live stage while generating, in place of the plain two lines. */
  progress?: ReactNode;
  /** A control at the end of one outstanding title's row. */
  titleAction?: (title: TitleDTO) => ReactNode;
  /** A title's state, in place of the plain badge. */
  titleBadge?: (title: TitleDTO) => ReactNode;
  /** An error about the outstanding titles, below their rows. */
  titlesError?: ReactNode;
};

export type { RequestDetailBodyProps };
