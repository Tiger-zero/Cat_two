import { useEffect, useLayoutEffect, useRef, useState } from "react";
import SidebarRow from "@/components/AppSidebar/SidebarRow";
import SidebarSection, { SIDEBAR_SECTION_STACK_CLASSES } from "@/components/AppSidebar/SidebarSection";
import { useInstance } from "@/contexts/InstanceContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { User_Role } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";
import { DEFAULT_SETTING_SECTION, SETTINGS_SECTIONS, type SettingSectionDefinition } from "./settingSections";

const basicSections = SETTINGS_SECTIONS.filter((section) => section.scope === "basic");
const adminSections = SETTINGS_SECTIONS.filter((section) => section.scope === "admin");
const adminPreloadKeys = [...new Set(adminSections.flatMap((section) => section.preloadSettingKeys ?? []))];

export default function SettingsContent({ initialSection }: { initialSection: string | null }) {
  const t = useTranslate();
  const user = useCurrentUser();
  const { fetchSettings } = useInstance();
  const isAdmin = user?.role === User_Role.ADMIN;
  const [selectedKey, setSelectedKey] = useState(initialSection || DEFAULT_SETTING_SECTION);
  const visibleSections = isAdmin ? SETTINGS_SECTIONS : basicSections;
  const activeSection = visibleSections.find((section) => section.key === selectedKey) ?? basicSections[0];
  const ActiveSection = activeSection.component;
  const contentRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (isAdmin) void fetchSettings(adminPreloadKeys);
  }, [fetchSettings, isAdmin]);

  useLayoutEffect(() => {
    if (contentRef.current) contentRef.current.scrollTop = 0;
  }, [activeSection.key]);

  const renderSections = (sections: SettingSectionDefinition[]) =>
    sections.map((section) => (
      <SidebarRow
        key={section.key}
        icon={section.icon}
        label={t(section.labelKey)}
        state={section.key === activeSection.key ? "current" : "idle"}
        onClick={() => setSelectedKey(section.key)}
      />
    ));

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[min(34%,13rem)_minmax(0,1fr)]">
      <nav
        aria-label={t("common.settings")}
        className="flex min-h-0 flex-col overflow-y-auto overscroll-contain border-e border-mu-line/70 bg-mu-well/55 p-3 sm:p-4"
      >
        <div className={SIDEBAR_SECTION_STACK_CLASSES}>
          <SidebarSection label={t("common.basic")}>{renderSections(basicSections)}</SidebarSection>
          {isAdmin && <SidebarSection label={t("common.admin")}>{renderSections(adminSections)}</SidebarSection>}
        </div>
      </nav>
      <section
        ref={contentRef}
        aria-label={t(activeSection.labelKey)}
        className="min-h-0 min-w-0 overflow-y-auto overscroll-contain px-4 pt-5 pb-6 sm:px-7 sm:pt-6 sm:pb-8"
      >
        <ActiveSection key={activeSection.key} />
      </section>
    </div>
  );
}
