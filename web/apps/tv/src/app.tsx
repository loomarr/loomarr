import { LoomarrProvider } from "@loomarr/design-system";
import { ClientPlatformProof } from "@loomarr/ui";
import { StatusBar } from "expo-status-bar";

const App = () => (
  <LoomarrProvider>
    <ClientPlatformProof />
    <StatusBar hidden />
  </LoomarrProvider>
);

export default App;
