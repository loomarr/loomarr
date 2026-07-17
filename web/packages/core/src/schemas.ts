// Zod validation schemas (frontend-design §4.3). They live in packages/core so the
// web app (react-hook-form) and the future Expo app reuse the exact same
// validation. Field shapes mirror the BE suggest.Intent (internal/suggest).

import { z } from "zod";

// The channel-intent form behind IntentInput (§3). description is the only
// required field — the blank-page killer templates prefill the rest.
export const intentSchema = z.object({
  description: z.string().trim().min(3, "Describe the channel you want (a sentence is plenty)."),
  era: z.string().trim().optional(),
  tone: z.string().trim().optional(),
  runtimeTarget: z.number().int().positive().optional(),
  maxAcquire: z.number().int().min(0).max(200).optional(),
  mustInclude: z.array(z.string().trim()).optional(),
  mustExclude: z.array(z.string().trim()).optional(),
});
export type IntentInput = z.infer<typeof intentSchema>;

// Local-admin bootstrap (wizard step 1, §13). Mirrors POST /v1/setup/bootstrap.
export const bootstrapSchema = z
  .object({
    username: z.string().trim().min(1, "Pick a username."),
    password: z.string().min(8, "At least 8 characters."),
    confirm: z.string(),
  })
  .refine((v) => v.password === v.confirm, { message: "Passwords don't match.", path: ["confirm"] });
export type BootstrapInput = z.infer<typeof bootstrapSchema>;

// Sign-in (local or imported media-server credentials, §11).
export const loginSchema = z.object({
  username: z.string().trim().min(1, "Enter your username."),
  password: z.string().min(1, "Enter your password."),
});
export type LoginInput = z.infer<typeof loginSchema>;
