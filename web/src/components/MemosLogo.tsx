import { useInstance } from "@/contexts/InstanceContext";
import { DEFAULT_LOGO, getCustomLogoUrl } from "@/lib/branding";
import { cn } from "@/lib/utils";
import IdentityLabel from "./IdentityLabel";

interface Props {
  className?: string;
  collapsed?: boolean;
  as?: "div" | "h1";
  /** Navigation uses header/md; auth pages use lg and About uses hero. */
  size?: keyof typeof LOGO_SCALE;
}

const LOGO_SCALE = {
  sm: { mark: 20, text: "sm" },
  header: { mark: 24, text: "md" },
  md: { mark: 28, text: "md" },
  lg: { mark: 32, text: "lg" },
  hero: { mark: 48, text: "hero" },
} as const;

function MemosLogo({ collapsed, size = "md", as: Component = "div", className }: Props) {
  const scale = LOGO_SCALE[size];
  const { generalSetting: instanceGeneralSetting } = useInstance();
  const title = instanceGeneralSetting.customProfile?.title || "Memos";
  const logoUrl = getCustomLogoUrl(instanceGeneralSetting.customProfile?.logoUrl);
  const markWidth = logoUrl || collapsed ? scale.mark : Math.ceil((scale.mark * 88) / 144);

  return (
    <Component className={cn("memos-logo w-fit min-w-0 max-w-full", className)}>
      <IdentityLabel
        label={title}
        size={scale.text}
        collapsed={collapsed}
        wrap={size === "lg" || size === "hero"}
        mark={
          <img
            src={logoUrl ?? DEFAULT_LOGO}
            width={markWidth}
            height={scale.mark}
            style={{ width: markWidth, height: scale.mark }}
            className={cn("shrink-0 select-none object-contain", !logoUrl && "memos-mark")}
            alt={collapsed ? title : ""}
            draggable={false}
            decoding="async"
          />
        }
      />
    </Component>
  );
}

export default MemosLogo;
