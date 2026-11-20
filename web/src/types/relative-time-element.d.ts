import type { Format, RelativeTimeElement } from "@github/relative-time-element";
import type { HTMLAttributes } from "react";

declare module "react" {
  // biome-ignore lint/style/noNamespace: React exposes JSX customization through namespace merging.
  namespace JSX {
    interface IntrinsicElements {
      "relative-time": HTMLAttributes<RelativeTimeElement> & {
        datetime?: string;
        format?: Format;
        "time-zone"?: string;
        "no-title"?: string;
      };
    }
  }
}
