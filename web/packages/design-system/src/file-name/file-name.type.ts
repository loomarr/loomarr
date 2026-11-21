import type { Density } from "../tokens";

interface FileNameProps {
  density?: Density;
  /**
   * `inline`, the default, sits in a line of text and shortens when its parent bounds its width.
   * `fill` is a block that takes the width its parent gives and asks for none: use it in a table
   * cell, where an inline name would hold the column open at its full length.
   */
  layout?: "fill" | "inline";
  /** The whole file name, as the library holds it ("Beacon.Moor.1980.1080p.mkv"). */
  name: string;
  /** The role's size at weight 600: the name that heads a row. */
  strong?: boolean;
  tone?: "primary" | "secondary";
}

export type { FileNameProps };
