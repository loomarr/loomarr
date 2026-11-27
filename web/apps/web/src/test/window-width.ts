import { act } from "@testing-library/react";

// The window's width as react-native-web reads it (the root element's clientWidth, which jsdom
// leaves at 0), so a layout that changes with the width lays out as it does on a desktop.
const setWindowWidth = (width: number) => {
  window.innerWidth = width;
  Object.defineProperty(document.documentElement, "clientWidth", { configurable: true, value: width });
  act(() => {
    window.dispatchEvent(new Event("resize"));
  });
};

export { setWindowWidth };
