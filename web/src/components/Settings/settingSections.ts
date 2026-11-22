import {
  ArrowLeftRightIcon,
  BellIcon,
  CogIcon,
  DatabaseIcon,
  KeyRoundIcon,
  type LucideIcon,
  NetworkIcon,
  Settings2Icon,
  ShieldIcon,
  SquarePenIcon,
  TagsIcon,
  UserIcon,
} from "lucide-react";
import { type ComponentType } from "react";
import AccessTokenSection from "@/components/Settings/AccessTokenSection";
import EditorSection from "@/components/Settings/EditorSection";
import InstanceSection from "@/components/Settings/InstanceSection";
import MemoExportSection from "@/components/Settings/MemoExportSection";
import MyAccountSection from "@/components/Settings/MyAccountSection";
import NetworkSection from "@/components/Settings/NetworkSection";
import NotificationSection from "@/components/Settings/NotificationSection";
import PreferencesSection from "@/components/Settings/PreferencesSection";
import SecuritySection from "@/components/Settings/SecuritySection";
import StorageSection from "@/components/Settings/StorageSection";
import TagsSection from "@/components/Settings/TagsSection";

export type SettingSectionKey =
  | "my-account"
  | "memo-export"
  | "access-token"
  | "preference"
  | "editor"
  | "system"
  | "security"
  | "notification"
  | "network"
  | "storage"
  | "tags";

type SettingSectionScope = "basic" | "admin";

export interface SettingSectionDefinition {
  key: SettingSectionKey;
  scope: SettingSectionScope;
  labelKey: `setting.${SettingSectionKey}.label`;
  icon: LucideIcon;
  component: ComponentType;
}

export const SETTINGS_SECTIONS: SettingSectionDefinition[] = [
  {
    key: "my-account",
    scope: "basic",
    labelKey: "setting.my-account.label",
    icon: UserIcon,
    component: MyAccountSection,
  },
  {
    key: "access-token",
    scope: "basic",
    labelKey: "setting.access-token.label",
    icon: KeyRoundIcon,
    component: AccessTokenSection,
  },
  {
    key: "preference",
    scope: "basic",
    labelKey: "setting.preference.label",
    icon: CogIcon,
    component: PreferencesSection,
  },
  {
    key: "editor",
    scope: "basic",
    labelKey: "setting.editor.label",
    icon: SquarePenIcon,
    component: EditorSection,
  },
  {
    key: "system",
    scope: "admin",
    labelKey: "setting.system.label",
    icon: Settings2Icon,
    component: InstanceSection,
  },
  {
    key: "security",
    scope: "admin",
    labelKey: "setting.security.label",
    icon: ShieldIcon,
    component: SecuritySection,
  },
  {
    key: "notification",
    scope: "admin",
    labelKey: "setting.notification.label",
    icon: BellIcon,
    component: NotificationSection,
  },
  {
    key: "network",
    scope: "admin",
    labelKey: "setting.network.label",
    icon: NetworkIcon,
    component: NetworkSection,
  },
  {
    key: "tags",
    scope: "basic",
    labelKey: "setting.tags.label",
    icon: TagsIcon,
    component: TagsSection,
  },
  {
    key: "memo-export",
    scope: "basic",
    labelKey: "setting.memo-export.label",
    icon: ArrowLeftRightIcon,
    component: MemoExportSection,
  },
  {
    key: "storage",
    scope: "admin",
    labelKey: "setting.storage.label",
    icon: DatabaseIcon,
    component: StorageSection,
  },
];

export const DEFAULT_SETTING_SECTION: SettingSectionKey = "my-account";

export const isSettingSectionKey = (value: string): value is SettingSectionKey => {
  return SETTINGS_SECTIONS.some((section) => section.key === value);
};

export function resolveSettingSectionKey(value: string | null): SettingSectionKey {
  if (value === "resource-stats") return "storage";
  return value && isSettingSectionKey(value) ? value : DEFAULT_SETTING_SECTION;
}
