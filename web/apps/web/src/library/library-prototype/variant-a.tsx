// PROTOTYPE — throwaway (#2151 step 2). Variant A — placeholder, replaced by its builder.
import { Text } from "@loomarr/design-system";
import type { VariantProps } from "./prototype-state";

const VariantA = ({ screen }: VariantProps) => <Text textRole="body">Variant A · {screen}</Text>;

export { VariantA };
