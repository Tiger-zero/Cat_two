import { useLayoutEffect, useRef, useState } from "react";
import SidebarRow from "@/components/AppSidebar/SidebarRow";
import SidebarSection from "@/components/AppSidebar/SidebarSection";
import useCurrentUser from "@/hooks/useCurrentUser";
import { User_Role } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";
import { resolveSettingSectionKey, SETTINGS_SECTIONS, type SettingSectionDefinition } from "./settingSections";

const basicSections = SETTINGS_SECTIONS.filter((section) => section.scope === "basic");
const adminSections = SETTINGS_SECTIONS.filter((section) => section.scope === "admin");

export default function SettingsContent({ initialSection }: { initialSection: string | null }) {
  const t = useTranslate();
  const user = useCurrentUser();
  const isAdmin = user?.role === User_Role.ADMIN;
  const [selectedKey, setSelectedKey] = useState(() => resolveSettingSectionKey(initialSection));
  const visibleSections = isAdmin ? SETTINGS_SECTIONS : basicSections;
  const activeSection = visibleSections.find((section) => section.key === selectedKey) ?? basicSections[0];
  const ActiveSection = activeSection.component;
  const contentRef = useRef<HTMLElement>(null);

  useLayoutEffect(() => {
    if (contentRef.current) contentRef.current.scrollTop = 0;
  }, [activeSection.key]);

  const renderSections = (sections: SettingSectionDefinition[]) =>
    sections.map((section) => (
      <SidebarRow
        key={section.key}
        icon={section.icon}
        label={t(section.labelKey)}
        state={section.key === activeSection.key ? "checked" : "idle"}
        onClick={() => setSelectedKey(section.key)}
      />
    ));

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[min(34%,14rem)_minmax(0,1fr)]">
      <nav
        aria-label={t("common.settings")}
        className="min-h-0 overflow-y-auto overscroll-contain border-e border-mu-line/70 bg-[color:var(--memos-sidebar,var(--mu-canvas))] px-2 py-4 sm:px-4 sm:py-6"
      >
        <div className="flex flex-col gap-5">
          <SidebarSection label={t("common.basic")}>{renderSections(basicSections)}</SidebarSection>
          {isAdmin && <SidebarSection label={t("common.admin")}>{renderSections(adminSections)}</SidebarSection>}
        </div>
      </nav>
      <section
        ref={contentRef}
        aria-label={t(activeSection.labelKey)}
        className="min-h-0 min-w-0 overflow-y-auto overscroll-contain px-4 py-6 sm:px-8 sm:py-8 lg:px-10"
      >
        <ActiveSection key={activeSection.key} />
      </section>
    </div>
  );
}
