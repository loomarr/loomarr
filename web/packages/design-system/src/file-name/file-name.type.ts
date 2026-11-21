import type { Density } from "../tokens";

interface FileNameProps {
  density?: Density;
  /** The whole file name, as the library holds it ("Beacon.Moor.1980.1080p.mkv"). */
  name: string;
  /** The role's size at weight 600: the name that heads a row. */
  strong?: boolean;
  tone?: "primary" | "secondary";
}

export type { FileNameProps };
