// Every orval response is a discriminated union on a literal status — `{ data: Body;
// status: 200 } | { data: ErrorModel; status: Exclude<HTTPStatusCodes, 200> }` — so
// reading the body means narrowing to 200 first. Done by hand that is:
//
//   const list = jobs.data?.status === 200 ? (jobs.data.data.jobs ?? []) : [];
//
// which appeared 78 times across 44 files before this helper existed. The repetition is
// the small problem. The real one is that the fallback is re-decided at every site, so
// the same endpoint could yield `[]` on one page and `undefined` on another, and a
// reader cannot tell whether that difference is deliberate.

/** An orval response: a discriminated union whose success arm is `status: 200`. */
type ApiResponse = { data: unknown; status: number };

/** The success arm of an orval response union. */
type Ok<R extends ApiResponse> = Extract<R, { status: 200 }>;

/**
 * Narrows an orval response to its 200 arm.
 *
 * Use when the union itself is what you hold — typically a mutation result inside
 * `onSuccess`, where the caller wants the whole response rather than a projection:
 *
 *   onSuccess: (res) => isOk(res) && setJobId(res.data.jobId)
 */
const isOk = <R extends ApiResponse>(res: R | undefined): res is Ok<R> => res?.status === 200;

/**
 * Reads the body of a settled query, or `undefined` while it is loading or errored.
 *
 *   const status = unwrap(llm.data);          // LlmStatusOutputBody | undefined
 *
 * `select` projects the body in the same step, which is the common case — the callers
 * that motivated this helper nearly all reach one field deep:
 *
 *   const list = unwrap(jobs.data, (b) => b.jobs) ?? [];
 *
 * The fallback stays at the call site deliberately. `?? []` and `?? undefined` mean
 * different things to a renderer (an empty list vs. nothing to render yet), and baking
 * one in would silently pick for every caller.
 */
// ⚠ A `function` declaration, against the arrow-function convention, because overload
// signatures have no arrow form: the two-arity contract (body, or body projected by
// `select`) is what lets callers write `(b) => b.jobs` with `b` inferred and unannotated.
// An arrow with a union return type would push that annotation to all 54 call sites.
function unwrap<R extends ApiResponse>(res: R | undefined): Ok<R>["data"] | undefined;
function unwrap<R extends ApiResponse, T>(
  res: R | undefined,
  select: (body: Ok<R>["data"]) => T,
): T | undefined;
function unwrap<R extends ApiResponse, T>(res: R | undefined, select?: (body: Ok<R>["data"]) => T) {
  if (!isOk(res)) return undefined;
  const body = res.data as Ok<R>["data"];
  return select ? select(body) : body;
}

export { isOk, unwrap };
