import { Button, Dialog, Spinner } from "@usememos/ui";
import { XIcon } from "lucide-react";
import { Suspense, useRef } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { useAuth } from "@/contexts/AuthContext";
import { useInstance } from "@/contexts/InstanceContext";
import { SETTINGS_SECTION_PARAM } from "@/router/routes";
import { useTranslate } from "@/utils/i18n";
import { lazyWithReload } from "@/utils/lazy";

const SettingsContent = lazyWithReload(() => import("./SettingsContent"));

export default function SettingsDialog() {
  const t = useTranslate();
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { settingsOpen, settingsReturnFocus, closeSettings } = useAppSidebar();
  const { currentUser, isInitialized: authInitialized } = useAuth();
  const { isInitialized: instanceInitialized } = useInstance();
  const closeRef = useRef<HTMLButtonElement>(null);
  const linkedSection = searchParams.get(SETTINGS_SECTION_PARAM);
  const open = Boolean(currentUser && authInitialized && instanceInitialized && (settingsOpen || linkedSection !== null));

  const onOpenChange = (nextOpen: boolean) => {
    if (nextOpen) return;
    closeSettings();
    if (linkedSection === null) return;

    const updated = new URLSearchParams(searchParams);
    updated.delete(SETTINGS_SECTION_PARAM);
    const { pathname, hash, state } = location;
    void navigate({ pathname, hash, search: updated.toString() }, { replace: true, preventScrollReset: true, state });
  };

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Popup
        variant="bare"
        sheet={false}
        initialFocus={closeRef}
        finalFocus={settingsReturnFocus}
        className="flex h-[min(44rem,calc(100dvh-2rem))] w-[min(62rem,calc(100vw-2rem))] flex-col"
      >
        <div className="flex shrink-0 items-center justify-between gap-4 border-b border-mu-line/70 px-4 py-2.5 sm:px-5">
          <Dialog.Title className="mu-label text-mu-text">{t("common.settings")}</Dialog.Title>
          <Dialog.Close render={<Button ref={closeRef} variant="ghost" size="sm" icon={<XIcon />} aria-label={t("common.close")} />} />
        </div>
        <Suspense
          fallback={
            <div className="grid flex-1 place-items-center" aria-busy="true">
              <Spinner />
            </div>
          }
        >
          <SettingsContent initialSection={linkedSection} />
        </Suspense>
      </Dialog.Popup>
    </Dialog.Root>
  );
}
