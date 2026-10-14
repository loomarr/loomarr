import type { useRender } from "@base-ui/react/use-render";

type ButtonVariant = "default" | "suggest" | "destructive" | "outline" | "secondary" | "ghost" | "link";
type ButtonSize = "default" | "sm" | "lg" | "icon";

// ⚠ The old Radix composition prop is GONE — it is `render` now (retired-ok), and the rename is
// deliberate rather than cosmetic. The old name described its mechanism: `Slot` merged props onto the
// single CHILD. Base UI takes the element as a PROP instead, so keeping a name that promises a
// child would be exactly the half-migrated vocabulary that outlives whoever introduced it. Every
// composed trigger in the app now reads the same way:
//
//   <Button render={<Link to="/filler" />}>Add clips</Button>
//
// (Base UI's own components carry a `nativeButton` escape hatch for this case. This Button is
// built directly on `useRender`, not on Base UI's Button, so there is nothing assuming button
// semantics to switch off — `render` alone is the whole contract.)
//
// @loomarr/design-system's Action gained these same six variants and its own web `render`
// escape hatch in the same change (#970 PR B checkpoint 3) — but Action's `render` chains
// handlers and concatenates `style`/`className` (Base UI's own `mergeProps` contract, which this
// Button already uses), not tailwind-merge's *override* semantics this component's own
// `buttonVariants()` depends on for its 11 external className consumers, so it cannot back this
// Button directly. See button.tsx's top comment.
interface ButtonProps extends useRender.ComponentProps<"button">, ButtonVariantProps {}

interface ButtonVariantProps {
  size?: ButtonSize | null;
  variant?: ButtonVariant | null;
}

export type { ButtonProps, ButtonSize, ButtonVariant };
