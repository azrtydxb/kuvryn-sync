import emblemDark from "./assets/kuvryn-sync-emblem-dark.png";
import emblemLight from "./assets/kuvryn-sync-emblem-light.png";

/** The Kuvryn Sync emblem for dark and light themes, 640x640 PNGs. */
export const EMBLEM_DARK = emblemDark;
export const EMBLEM_LIGHT = emblemLight;

/** ProductLogo props shared by every Kuvryn Sync lockup. */
export const PRODUCT = {
  name: "Kuvryn",
  sub: "Sync",
  tagline: "GitOps that sticks",
  pillar: "build",
  emblem: emblemDark,
  emblemLight: emblemLight,
} as const;
