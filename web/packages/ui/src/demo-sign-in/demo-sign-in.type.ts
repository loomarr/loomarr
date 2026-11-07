type DemoSignInProps = {
  /** The demo account the server offers, shown on the button so nobody has to type it. */
  username: string;
  /** Sign in as that account. */
  onSignIn: () => void;
  /** True while that sign-in is in flight. */
  busy?: boolean;
};

export type { DemoSignInProps };
