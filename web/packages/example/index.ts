import { greetingFor } from "./src/lib";

const welcomeViewer = (name: string): string => greetingFor(name);

export { welcomeViewer };
