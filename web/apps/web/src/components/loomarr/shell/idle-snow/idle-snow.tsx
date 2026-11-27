import { AnalogSnow, useReducedMotionPreference } from "@loomarr/design-system";

// IdleSnow: the faint snow behind the entry surfaces (login, the wizard), and nowhere near data,
// tables or forms (§1, #2789). It fills its parent behind positioned content and is inert.
//
// Nothing is drawn until motion is known to be allowed: under reduced motion these surfaces carry no
// snow at all, which also keeps their screenshot baselines free of noise (§5.2, §17).
const IdleSnow = () => {
  const reducedMotion = useReducedMotionPreference();
  return reducedMotion === false ? <AnalogSnow reducedMotion={false} /> : null;
};

export { IdleSnow };
