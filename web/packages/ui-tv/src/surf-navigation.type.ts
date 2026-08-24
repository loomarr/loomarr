import type { SurfSelection } from "@loomarr/ui";

type TvSurfDirection = "down" | "up";

type TvSurfMoveResult = {
  boundary?: TvSurfDirection;
  selection: SurfSelection;
};

type TvSurfActivation = {
  channelId: string;
  kind: "tune";
};

export type { TvSurfActivation, TvSurfDirection, TvSurfMoveResult };
