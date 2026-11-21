import type { Density } from "../tokens";

interface FileNameProps {
  density?: Density;
  /**
   * `inline`, the default, sits in a line of text and shortens when its parent bounds its width.
   * `fill` is for a table cell only: it takes the width the cell gives and asks for none, where an
   * inline name would hold the column open at its full length. In a parent sized to its content it
   * has no width at all.
   */
  layout?: "fill" | "inline";
  /** The whole file name, as the library holds it ("Beacon.Moor.1980.1080p.mkv"). */
  name: string;
  /** The role's size at weight 600: the name that heads a row. */
  strong?: boolean;
  tone?: "primary" | "secondary";
}

export type { FileNameProps };
