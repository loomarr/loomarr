import type { z } from "zod";

// validated — parse a fixture through its GENERATED response schema and return it typed.
//
// This is the piece that makes a shared mock layer trustworthy, and it is the surviving half of a
// plan that changed. The original design put runtime validation in the transport (`customFetch`),
// so a response that did not match the spec would be caught in dev. That turned out to be
// unbuildable with this stack and not worth hand-rolling:
//
//   • orval 7.21.0 has no `runtimeValidation` at all — zero occurrences in any @orval/* package.
//   • orval 8.x has it, but the ONLY place it injects a `.parse()` is the Angular
//     `.pipe(map(...))` path; the fetch client's mutator branch returns before reaching it.
//   • Loomarr's transport IS a custom mutator (CSRF, cookie auth, RFC 7807), and orval PR #3226
//     — "pass zod schema to custom fetch response implementation" — is still open.
//
// ⚠ But the value was never catching BACKEND drift: `openapi-verify` already guarantees the spec
// matches the Go definitions, so a response that disagrees with the schema is close to impossible.
// The value was catching FIXTURE drift — test data written by hand that quietly stops resembling
// the wire. That needs no transport and no generated URL→schema map: a test knows which endpoint
// it is stubbing, so it can name the schema itself, and the failure lands in the test that owns
// the bad fixture rather than somewhere downstream.
//
// Usage:
//   import { listTitlesResponse } from "@loomarr/api/zod";
//   export const titles = validated(listTitlesResponse, { titles: [ … ] });
const validated = <S extends z.ZodTypeAny>(schema: S, value: z.input<S>): z.output<S> => {
  const result = schema.safeParse(value);
  if (!result.success) {
    // ⚠ Throw at MODULE LOAD, not inside a test. A fixture is shared, so a bad one should fail
    // loudly once with the field path rather than surfacing as an unrelated assertion failure in
    // whichever test happened to use it first.
    throw new Error(
      `fixture does not satisfy its generated wire schema:\n${JSON.stringify(result.error.issues, null, 2)}`,
    );
  }
  return result.data;
};

export { validated };
