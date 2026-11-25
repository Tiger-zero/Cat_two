import { create } from "@bufbuild/protobuf";
import { Button, Select } from "@usememos/ui";
import { useEffect, useMemo, useState } from "react";
import { useInstance } from "@/contexts/InstanceContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import useDialog from "@/hooks/useDialog";
import { DEFAULT_APP_NAME } from "@/lib/branding";
import { toast } from "@/lib/toast";
import { InstanceSetting_GeneralSettingSchema, InstanceSetting_Key, InstanceSettingSchema } from "@/types/proto/api/instance_service_pb";
import { User_Role } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";
import ChangeMemberPasswordDialog from "../ChangeMemberPasswordDialog";
import UpdateCustomizedProfileDialog from "../UpdateCustomizedProfileDialog";
import SettingGroup from "./SettingGroup";
import { SettingList, SettingListItem } from "./SettingList";
import SettingSection from "./SettingSection";
import useInstanceSettingUpdater, { buildInstanceSettingName } from "./useInstanceSettingUpdater";

const InstanceSection = () => {
  const t = useTranslate();
  const user = useCurrentUser();
  const admin = user?.role === User_Role.ADMIN ? user : undefined;
  const customizeDialog = useDialog();
  const passwordDialog = useDialog();
  const saveInstanceSetting = useInstanceSettingUpdater();
  const { generalSetting } = useInstance();
  const [weekStartDayOffset, setWeekStartDayOffset] = useState(generalSetting.weekStartDayOffset);
  useEffect(() => {
    setWeekStartDayOffset(generalSetting.weekStartDayOffset);
  }, [generalSetting.weekStartDayOffset]);

  const weekStartDayOptions = useMemo(
    () => [
      { value: "-1", label: t("setting.instance.saturday") },
      { value: "0", label: t("setting.instance.sunday") },
      { value: "1", label: t("setting.instance.monday") },
    ],
    [t],
  );

  const hasUnsavedChanges = weekStartDayOffset !== generalSetting.weekStartDayOffset;

  const handleSaveSettings = async () => {
    if (!hasUnsavedChanges) return;
    await saveInstanceSetting({
      key: InstanceSetting_Key.GENERAL,
      setting: create(InstanceSettingSchema, {
        name: buildInstanceSettingName(InstanceSetting_Key.GENERAL),
        value: {
          case: "generalSetting",
          value: create(InstanceSetting_GeneralSettingSchema, { ...generalSetting, weekStartDayOffset, additionalScript: "" }),
        },
      }),
      errorContext: "Update general settings",
    });
  };

  return (
    <SettingSection title={t("setting.system.label")}>
      <SettingGroup title={t("common.basic")} description={t("setting.system.basic-description")}>
        <SettingList>
          <SettingListItem label={t("setting.system.server-name")} description={generalSetting.customProfile?.title || DEFAULT_APP_NAME}>
            <Button variant="neutral" onClick={customizeDialog.open}>
              {t("common.edit")}
            </Button>
          </SettingListItem>

          <SettingListItem label={t("setting.instance.week-start-day")} description={t("setting.instance.week-start-day-description")}>
            <Select.Root
              value={weekStartDayOffset.toString()}
              items={weekStartDayOptions}
              onValueChange={(value) => setWeekStartDayOffset(Number(value))}
            >
              <Select.Trigger className="min-w-fit">
                <Select.Value />
              </Select.Trigger>
              <Select.Popup>
                {weekStartDayOptions.map((option) => (
                  <Select.Item key={option.value} value={option.value}>
                    {option.label}
                  </Select.Item>
                ))}
              </Select.Popup>
            </Select.Root>
          </SettingListItem>
        </SettingList>
        <div className="flex justify-end">
          <Button variant="primary" disabled={!hasUnsavedChanges} onClick={handleSaveSettings}>
            {t("common.save")}
          </Button>
        </div>
      </SettingGroup>

      {admin && (
        <SettingGroup title={t("setting.instance.access-title")} showSeparator>
          <SettingList>
            <SettingListItem label={t("common.password")} description={t("setting.instance.access-description")}>
              <Button variant="neutral" onClick={passwordDialog.open}>
                {t("setting.account.change-password")}
              </Button>
            </SettingListItem>
          </SettingList>
          <ChangeMemberPasswordDialog open={passwordDialog.isOpen} onOpenChange={passwordDialog.setOpen} user={admin} />
        </SettingGroup>
      )}

      <UpdateCustomizedProfileDialog
        open={customizeDialog.isOpen}
        onOpenChange={customizeDialog.setOpen}
        onSuccess={() => {
          toast.add({ title: t("message.update-succeed"), type: "success" });
        }}
      />
    </SettingSection>
  );
};

export default InstanceSection;
