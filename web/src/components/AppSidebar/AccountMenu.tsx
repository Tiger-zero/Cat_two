import { Popover } from "@usememos/ui";
import { ChevronsUpDownIcon } from "lucide-react";
import { useRef, useState } from "react";
import IdentityLabel from "@/components/IdentityLabel";
import MemosLogo from "@/components/MemosLogo";
import UserAvatar from "@/components/UserAvatar";
import UserMenu from "@/components/UserMenu";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";
import { sidebarSurfaceVariants } from "./sidebar-layout";

export default function AccountMenu({ className, size = "md" }: { className?: string; size?: "md" | "header" }) {
  const t = useTranslate();
  const user = useCurrentUser();
  const accountLabel = user?.displayName || user?.username;
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const pendingSettings = useRef(false);
  const { openSettings, closeMobileThen } = useAppSidebar();
  return (
    <Popover.Root
      open={open}
      onOpenChange={setOpen}
      onOpenChangeComplete={(nextOpen) => {
        // Let the account popover finish closing before Settings takes focus.
        if (nextOpen || !pendingSettings.current) return;
        pendingSettings.current = false;
        closeMobileThen(() => openSettings(triggerRef.current));
      }}
    >
      <Popover.Trigger
        ref={triggerRef}
        aria-label={`${t("common.account-menu")}: ${accountLabel ?? t("common.memos")}`}
        title={accountLabel}
        className={cn(
          "memos-brand-trigger border border-transparent text-start transition-colors hover:bg-mu-hover/65 mu-focus focus-visible:border-mu-focus data-popup-open:bg-mu-hover/75",
          sidebarSurfaceVariants({ role: size === "header" ? "headerBrand" : "mobileBrand" }),
          className,
        )}
      >
        {accountLabel ? (
          <IdentityLabel
            label={accountLabel}
            mark={
              <span className={cn("flex shrink-0 items-center justify-center", size === "header" ? "size-6" : "size-7")}>
                <UserAvatar avatarUrl={user?.avatarUrl} name={accountLabel} className={size === "header" ? "size-[22px]" : "size-[26px]"} />
              </span>
            }
          />
        ) : (
          <MemosLogo size={size} />
        )}
        <ChevronsUpDownIcon
          aria-hidden="true"
          className={cn(size === "header" ? "mu-icon-xs" : "mu-icon-sm", "shrink-0 text-mu-text-secondary/70")}
        />
      </Popover.Trigger>
      <Popover.Popup
        variant="list"
        align="start"
        positionMethod="fixed"
        side="bottom"
        className="mu-compact w-[min(15rem,calc(100vw-1rem))] max-h-[min(27rem,calc(100dvh-5rem))] overflow-y-auto"
      >
        <UserMenu
          onClose={() => setOpen(false)}
          onOpenSettings={() => {
            pendingSettings.current = true;
            setOpen(false);
          }}
        />
      </Popover.Popup>
    </Popover.Root>
  );
}
