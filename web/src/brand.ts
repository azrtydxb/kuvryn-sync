import emblemDark from "./assets/kuvryn-sync-emblem-dark.png";
import emblemLight from "./assets/kuvryn-sync-emblem-light.png";

/**
 * ProductLogo props shared by every Kuvryn Sync lockup, with the 640x640
 * emblems for the dark and light themes.
 */
export const PRODUCT = {
  name: "Kuvryn",
  sub: "Sync",
  tagline: "GitOps that sticks",
  pillar: "build",
  emblem: emblemDark,
  emblemLight: emblemLight,
} as const;
