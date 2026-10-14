import { Screen, Surface, Text } from "@loomarr/design-system";

const ClientPlatformProof = () => (
  <Screen alignItems="center" justifyContent="center">
    <Surface gap="$control" maxWidth={760} padding="$section" width="100%">
      <Text textRole="title">Loomarr</Text>
      <Text textRole="body">One product language, shaped for touch and ten-foot viewing.</Text>
    </Surface>
  </Screen>
);

export { ClientPlatformProof };
