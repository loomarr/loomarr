import type { DiscoveredServer } from "@loomarr/core/server-discovery";

type ServerRowProps = {
  onPress: () => void;
  server: DiscoveredServer;
};

export type { ServerRowProps };
