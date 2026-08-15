// Zod validation schemas (frontend-design §4.3). They live in packages/core so the
// web app (TanStack Form, via Standard Schema) and the future Expo app reuse the exact same
// validation.
//
// ⚠ **FIELD NAMES ARE DERIVED, NOT MIRRORED (§14).** Each schema is `.pick()`ed off the
// generated wire schema in `@loomarr/api/zod`, so a name that is not on the wire is a COMPILE
// error here rather than a value the server silently ignores. This schema once said
// `maxAcquire` where the wire says `maxAcquisitions`, and `runtimeTarget` where it says
// `runtimeTargetMin`: both parsed, both serialized to JSON the server dropped on the floor, and
// a user's acquisition cap and runtime target just vanished. Types alone did not catch it,
// because the form never had to satisfy `Intent`.
//
// ⚠ **The error message is cryptic — recognise it.** Picking a key that is not on the wire
// fails with `Type 'true' is not assignable to type 'never'` on that line. It means exactly
// one thing: *that field does not exist on the wire schema.*
//
// ⚠ **Generation carries NAMES and TYPES, not RULES.** The spec declares almost no constraints
// (5 `minimum`, 3 `maximum`, 7 `minLength` across ~9k lines; `maxAcquisitions` has no bounds at
// all), and OpenAPI has nowhere to put a user-facing message. So every product rule below —
// the trims, the lengths, the 0–200 cap, the password confirmation — is deliberately
// hand-authored and must STAY that way. Do not "simplify" these into the generated schema.
import { loginBody } from "@loomarr/api/zod/auth";
import { submitProposalBody } from "@loomarr/api/zod/proposals";
import { bootstrapBody } from "@loomarr/api/zod/setup";
import { z } from "zod";

// The channel-intent form behind IntentInput (§3). description is the only
// required field — the blank-page killer templates prefill the rest.
//
// ⚠ `.pick()` rather than using the wire schema directly: POST /v1/proposals also carries
// `adjacent`, `currentLineup` and `refineText`, which belong to the refine flow, not this form.
const intentShape = submitProposalBody.pick({
  description: true,
  era: true,
  tone: true,
  runtimeTargetMin: true,
  maxAcquisitions: true,
  mustInclude: true,
  mustExclude: true,
});

const intentSchema = intentShape.extend({
  description: z.string().trim().min(3, "Describe the channel you want (a sentence is plenty)."),
  era: z.string().trim().optional(),
  tone: z.string().trim().optional(),
  runtimeTargetMin: z.number().int().positive().optional(),
  maxAcquisitions: z.number().int().min(0).max(200).optional(),
  mustInclude: z.array(z.string().trim()).optional(),
  mustExclude: z.array(z.string().trim()).optional(),
});

// Local-admin bootstrap (wizard step 1, §13). Mirrors POST /v1/setup/bootstrap.
//
// ⚠ `confirm` is FORM-ONLY and correctly absent from the wire — the server never receives it.
// That is why the pattern is pick-then-extend rather than a straight derive: the wire decides
// which of these names are real, and the form is free to add its own on top.
const bootstrapSchema = bootstrapBody
  .pick({ username: true, password: true })
  .extend({
    username: z.string().trim().min(1, "Pick a username."),
    password: z.string().min(8, "At least 8 characters."),
    confirm: z.string(),
  })
  .refine((v) => v.password === v.confirm, { message: "Passwords don't match.", path: ["confirm"] });

// Sign-in (local or imported media-server credentials, §11).
const loginSchema = loginBody.pick({ username: true, password: true }).extend({
  username: z.string().trim().min(1, "Enter your username."),
  password: z.string().min(1, "Enter your password."),
});

export { bootstrapSchema, intentSchema, loginSchema };
