// PROTOTYPE — throwaway (#2151 step 2). Variant B — placeholder, replaced by its builder.
import { Text } from "@loomarr/design-system";
import type { VariantProps } from "./prototype-state";

const VariantB = ({ screen }: VariantProps) => <Text textRole="body">Variant B · {screen}</Text>;

export { VariantB };
