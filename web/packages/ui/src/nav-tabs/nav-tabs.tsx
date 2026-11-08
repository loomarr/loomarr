import type { NavTabsProps } from "./nav-tabs.type";

// NavTabs is the web's router-link tab bar. Native apps navigate with TabBar and their own stacks,
// so the native adapter refuses to render rather than drawing links that go nowhere.
const NavTabs = (_props: NavTabsProps): never => {
  throw new Error("NavTabs is web-only; native navigation uses TabBar");
};

export { NavTabs };
