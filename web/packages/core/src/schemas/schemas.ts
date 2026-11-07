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
// ⚠ **A limit the server declares is read from the generated constant; the MESSAGE stays here.**
// The password minimum, the description minimum and the acquisition cap are generated constants
// (`bootstrapBodyPasswordMin`, `submitProposalBodyDescriptionMin`, …) from the server's struct
// tags, so the form and the API cannot disagree about them. OpenAPI has nowhere to put a
// user-facing message, so every message is authored below. Rules the server does not declare —
// the trims, the password confirmation — are still hand-authored here; moving one to the server
// means a struct tag there and its generated constant here.
// ⚠ PascalCase because orval 8.24 renamed generated zod SCHEMA exports (`loginBody` →
// `LoginBody`). Scalar constraint constants kept their camelCase (`changePasswordBodyNextMin`),
// so the casing here is not a blanket rule — it tracks what the generator emits per symbol.
import {
  ChangePasswordBody,
  changePasswordBodyNextMin,
  DevicePairApproveBody,
  LoginBody,
  RedeemPasswordRecoveryBody,
  RequestPasswordRecoveryBody,
  redeemPasswordRecoveryBodyPasswordMin,
  requestPasswordRecoveryBodyUsernameMax,
} from "@loomarr/api/zod/auth";
import {
  CreateInvitationBody,
  createInvitationBodyContactEmailMax,
  createInvitationBodyLibraryUserIdMax,
  createInvitationBodyUsernameMax,
  RedeemInvitationBody,
  redeemInvitationBodyPasswordMin,
} from "@loomarr/api/zod/invitations";
import { NotificationProvidersUpdateBody } from "@loomarr/api/zod/notifications";
import {
  SubmitProposalBody,
  submitProposalBodyDescriptionMin,
  submitProposalBodyMaxAcquisitionsMax,
  submitProposalBodyMaxAcquisitionsMin,
} from "@loomarr/api/zod/proposals";
import { BootstrapBody, bootstrapBodyPasswordMin } from "@loomarr/api/zod/setup";
import {
  CreateLocalUserBody,
  createLocalUserBodyPasswordMin,
  PutUserContactAddressBody,
  putUserContactAddressBodyEmailMax,
  ResetUserPasswordBody,
  resetUserPasswordBodyNextMin,
} from "@loomarr/api/zod/users";
import { z } from "zod";

// The channel-intent form behind IntentInput (§3). description is the only
// required field — the blank-page killer templates prefill the rest.
//
// ⚠ `.pick()` rather than using the wire schema directly: POST /v1/proposals also carries
// `adjacent`, `currentLineup` and `refineText`, which belong to the refine flow, not this form.
const intentShape = SubmitProposalBody.pick({
  description: true,
  era: true,
  tone: true,
  runtimeTargetMin: true,
  maxAcquisitions: true,
  mustInclude: true,
  mustExclude: true,
});

const intentSchema = intentShape.extend({
  description: z
    .string()
    .trim()
    .min(submitProposalBodyDescriptionMin, "Describe the channel you want (a sentence is plenty)."),
  era: z.string().trim().optional(),
  tone: z.string().trim().optional(),
  // positive, not the spec's minimum of 0: the server reads 0 as "no target", which the form
  // expresses by leaving the field empty.
  runtimeTargetMin: z.number().int().positive().optional(),
  maxAcquisitions: z
    .number()
    .int()
    .min(submitProposalBodyMaxAcquisitionsMin)
    .max(submitProposalBodyMaxAcquisitionsMax)
    .optional(),
  mustInclude: z.array(z.string().trim()).optional(),
  mustExclude: z.array(z.string().trim()).optional(),
});

// Local-admin bootstrap (wizard step 1, §13). Mirrors POST /v1/setup/bootstrap.
//
// ⚠ `confirm` is FORM-ONLY and correctly absent from the wire — the server never receives it.
// That is why the pattern is pick-then-extend rather than a straight derive: the wire decides
// which of these names are real, and the form is free to add its own on top.
const bootstrapSchema = BootstrapBody.pick({ username: true, password: true })
  .extend({
    username: z.string().trim().min(1, "Pick a username."),
    password: z.string().min(bootstrapBodyPasswordMin, `At least ${bootstrapBodyPasswordMin} characters.`),
    confirm: z.string(),
  })
  .refine((v) => v.password === v.confirm, { message: "Passwords don't match.", path: ["confirm"] });

// Sign-in (local or imported media-server credentials, §11).
const loginSchema = LoginBody.pick({ username: true, password: true }).extend({
  username: z.string().trim().min(1, "Enter your username."),
  password: z.string().min(1, "Enter your password."),
});

const atLeast = (min: number) => `Use at least ${min} characters.`;

// Change your own password (Your account, §11). `confirm` is form-only, as in bootstrapSchema.
const changePasswordSchema = ChangePasswordBody.pick({ current: true, next: true })
  .extend({
    current: z.string().min(1, "Enter your current password."),
    next: z.string().min(changePasswordBodyNextMin, atLeast(changePasswordBodyNextMin)),
    confirm: z.string(),
  })
  .refine((v) => v.next === v.confirm, { message: "New passwords don't match.", path: ["confirm"] });

// An admin adds a local account (People). The role is the wire's own enum, required here because
// the form always shows a choice; the server's default (member) is the form's default value.
const createLocalUserSchema = CreateLocalUserBody.pick({ username: true, password: true, role: true }).extend(
  {
    username: z.string().trim().min(1, "Pick a username."),
    password: z.string().min(createLocalUserBodyPasswordMin, atLeast(createLocalUserBodyPasswordMin)),
    role: CreateLocalUserBody.shape.role.unwrap(),
  },
);

// Redeeming an invitation. A Library invitation signs in with the invited Emby or Jellyfin
// account, so it needs that username and checks the existing password; a local one creates a
// password, so it confirms it instead. One shape for both keeps one form: the field a path does
// not show is simply never checked.
const redeemInvitationSchema = (library: boolean) =>
  RedeemInvitationBody.pick({ username: true, password: true })
    .extend({
      username: library
        ? z.string().trim().min(1, "Enter the username for the invited Library account.")
        : z.string(),
      password: z.string().min(redeemInvitationBodyPasswordMin, atLeast(redeemInvitationBodyPasswordMin)),
      confirm: z.string(),
    })
    .refine((v) => library || v.password === v.confirm, {
      message: "Passwords need to match.",
      path: ["confirm"],
    });

// Forgot password: ask for a reset link by username.
const passwordRecoveryRequestSchema = RequestPasswordRecoveryBody.pick({ username: true }).extend({
  username: z
    .string()
    .trim()
    .min(1, "Enter your username.")
    .max(
      requestPasswordRecoveryBodyUsernameMax,
      `Use ${requestPasswordRecoveryBodyUsernameMax} characters or fewer.`,
    ),
});

// Redeem a reset link with a new password.
const passwordRecoveryResetSchema = RedeemPasswordRecoveryBody.pick({ password: true })
  .extend({
    password: z
      .string()
      .min(redeemPasswordRecoveryBodyPasswordMin, atLeast(redeemPasswordRecoveryBodyPasswordMin)),
    confirm: z.string(),
  })
  .refine((v) => v.password === v.confirm, { message: "Passwords need to match.", path: ["confirm"] });

// An admin sets a replacement password for a local account.
const resetUserPasswordSchema = ResetUserPasswordBody.pick({ next: true }).extend({
  next: z.string().min(resetUserPasswordBodyNextMin, atLeast(resetUserPasswordBodyNextMin)),
});

// A person's contact email. Deliberately looser than z.email(): the server proves the mailbox by
// delivering to it, so the form only catches an obviously incomplete address.
const completeEmail = (email: string) =>
  email.includes("@") && !email.startsWith("@") && !email.endsWith("@");
const contactAddressSchema = PutUserContactAddressBody.pick({ email: true }).extend({
  email: z
    .string()
    .trim()
    .max(putUserContactAddressBodyEmailMax, `Use ${putUserContactAddressBodyEmailMax} characters or fewer.`)
    .refine(completeEmail, "Enter one complete email address."),
});

// An admin reserves an invitation (People). A local one reserves a username; a Library one picks
// an Emby or Jellyfin account. One shape for both keeps one form: the field a path does not show
// is simply never checked. The contact email is optional, so a blank one passes. `sendEmail` is
// form-only (the "Also send by email" choice), as `confirm` is in bootstrapSchema.
const createInvitationSchema = CreateInvitationBody.pick({
  kind: true,
  username: true,
  libraryUserId: true,
  contactEmail: true,
  role: true,
})
  .extend({
    username: z.string().trim().max(createInvitationBodyUsernameMax),
    libraryUserId: z.string().max(createInvitationBodyLibraryUserIdMax),
    contactEmail: z
      .string()
      .trim()
      .max(
        createInvitationBodyContactEmailMax,
        `Use ${createInvitationBodyContactEmailMax} characters or fewer.`,
      )
      .refine((email) => email === "" || completeEmail(email), "Enter one complete email address."),
    role: CreateInvitationBody.shape.role.unwrap(),
    sendEmail: z.boolean(),
  })
  // `when` keeps these running beside a bad email, so every missing field is reported together.
  .refine((v) => v.kind !== "local" || v.username !== "", {
    message: "Choose a username to reserve.",
    path: ["username"],
    when: () => true,
  })
  .refine((v) => v.kind !== "library" || v.libraryUserId !== "", {
    message: "Choose an Emby or Jellyfin account.",
    path: ["libraryUserId"],
    when: () => true,
  });

// Approving a device's pairing code. A blank code is only ever blocked: the submit button is
// disabled until something is typed, so no message is written for it.
const devicePairApproveSchema = DevicePairApproveBody.pick({ userCode: true }).extend({
  userCode: z.string().trim().min(1),
});

interface NotificationProviderRequiredField {
  key: string;
  label: string;
}

// Notification providers share one dynamic form. The server supplies provider-specific fields,
// while this schema owns the stable label/events/settings contract. `clearedSecrets` is form-only:
// it lets validation distinguish an intentionally cleared required secret from an omitted secret
// that the update endpoint must preserve.
const notificationProviderFormBase = NotificationProvidersUpdateBody.pick({
  enabled: true,
  events: true,
  label: true,
  settings: true,
}).extend({
  label: z.string().trim().min(1, "Enter a label.").max(120, "Use 120 characters or fewer."),
  events: z.array(z.string()).min(1, "Choose at least one event."),
  settings: z.record(z.string(), z.string()),
  clearedSecrets: z.array(z.string()),
});

const notificationProviderFormSchema = (
  requiredFields: NotificationProviderRequiredField[],
  configuredSecretKeys: string[] = [],
) => {
  const configured = new Set(configuredSecretKeys);
  return notificationProviderFormBase.superRefine((value, context) => {
    for (const field of requiredFields) {
      const supplied = (value.settings[field.key] ?? "").trim() !== "";
      const preserved = configured.has(field.key) && !value.clearedSecrets.includes(field.key);
      if (!supplied && !preserved) {
        context.addIssue({
          code: "custom",
          message: `Enter ${field.label}.`,
          path: ["settings", field.key],
        });
      }
    }
  });
};

export type { NotificationProviderRequiredField };
export {
  bootstrapSchema,
  changePasswordSchema,
  contactAddressSchema,
  createInvitationSchema,
  createLocalUserSchema,
  devicePairApproveSchema,
  intentSchema,
  loginSchema,
  notificationProviderFormSchema,
  passwordRecoveryRequestSchema,
  passwordRecoveryResetSchema,
  redeemInvitationSchema,
  resetUserPasswordSchema,
};
