interface ProblemDetail {
  type?: string;
  title?: string;
  status?: number;
  detail?: string;
  // huma attaches field-level errors here for 422s.
  errors?: { message: string; location?: string; value?: unknown }[];
}

export type { ProblemDetail };
