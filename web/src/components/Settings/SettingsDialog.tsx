import { Button, Dialog, Spinner } from "@usememos/ui";
import { Settings2Icon, XIcon } from "lucide-react";
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
        className="h-[min(46rem,calc(100dvh-3rem))] w-[min(66rem,calc(100vw-2rem))] bg-mu-surface"
      >
        <header className="flex shrink-0 items-center justify-between gap-4 border-b border-mu-line/70 px-4 py-3 sm:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <span className="grid size-8 shrink-0 place-items-center rounded-mu-lg bg-mu-selected text-mu-primary" aria-hidden="true">
              <Settings2Icon className="mu-icon" />
            </span>
            <Dialog.Title className="mu-title">{t("common.settings")}</Dialog.Title>
          </div>
          <Dialog.Close render={<Button ref={closeRef} variant="ghost" size="sm" icon={<XIcon />} aria-label={t("common.close")} />} />
        </header>
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
