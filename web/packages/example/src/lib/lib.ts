const greetingFor = (name: string): string => {
  const viewer = name.trim() || "viewer";
  return `Welcome, ${viewer}.`;
};

export { greetingFor };
