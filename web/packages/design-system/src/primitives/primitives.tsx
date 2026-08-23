import { styled, Text, View } from "@tamagui/core";

const Screen = styled(View, {
  name: "LoomarrScreen",
  flex: 1,
  width: "100%",
  minHeight: "100%",
  alignItems: "center",
  justifyContent: "center",
  backgroundColor: "$surfaceCanvas",
  padding: "$screen",
});

const Panel = styled(View, {
  name: "LoomarrPanel",
  width: "100%",
  maxWidth: 760,
  gap: "$control",
  backgroundColor: "$surfaceRaised",
  borderRadius: "$overlay",
  padding: "$section",
});

const Heading = styled(Text, {
  name: "LoomarrHeading",
  color: "$contentPrimary",
  fontSize: 32,
  fontWeight: "700",
  lineHeight: 40,
});

const Body = styled(Text, {
  name: "LoomarrBody",
  color: "$contentSecondary",
  fontSize: 18,
  lineHeight: 26,
});

export { Body, Heading, Panel, Screen };
