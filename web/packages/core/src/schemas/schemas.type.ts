import type { z } from "zod";
import type { bootstrapSchema, intentSchema, loginSchema } from "./schemas";

type IntentInput = z.infer<typeof intentSchema>;
type BootstrapInput = z.infer<typeof bootstrapSchema>;
type LoginInput = z.infer<typeof loginSchema>;

export type { BootstrapInput, IntentInput, LoginInput };
