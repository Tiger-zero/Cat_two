import { create } from "@bufbuild/protobuf";
import { Button, Select } from "@usememos/ui";
import { isEqual } from "lodash-es";
import { useEffect, useMemo, useState } from "react";
import { useInstance } from "@/contexts/InstanceContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import useDialog from "@/hooks/useDialog";
import { DEFAULT_APP_NAME } from "@/lib/branding";
import { toast } from "@/lib/toast";
import {
  InstanceSetting_GeneralSetting,
  InstanceSetting_GeneralSettingSchema,
  InstanceSetting_Key,
  InstanceSettingSchema,
} from "@/types/proto/api/instance_service_pb";
import { User_Role } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";
import ChangeMemberPasswordDialog from "../ChangeMemberPasswordDialog";
import UpdateCustomizedProfileDialog from "../UpdateCustomizedProfileDialog";
import SettingGroup from "./SettingGroup";
import { SettingCodeEditor, SettingList, SettingListItem } from "./SettingList";
import SettingSection from "./SettingSection";
import useInstanceSettingUpdater, { buildInstanceSettingName } from "./useInstanceSettingUpdater";

const InstanceSection = () => {
  const t = useTranslate();
  const user = useCurrentUser();
  const admin = user?.role === User_Role.ADMIN ? user : undefined;
  const customizeDialog = useDialog();
  const passwordDialog = useDialog();
  const saveInstanceSetting = useInstanceSettingUpdater();
  const { generalSetting: originalGeneralSetting } = useInstance();
  const [instanceGeneralSetting, setInstanceGeneralSetting] = useState<InstanceSetting_GeneralSetting>(originalGeneralSetting);

  useEffect(() => {
    setInstanceGeneralSetting(originalGeneralSetting);
  }, [originalGeneralSetting]);

  const weekStartDayOptions = useMemo(
    () => [
      { value: "-1", label: t("setting.instance.saturday") },
      { value: "0", label: t("setting.instance.sunday") },
      { value: "1", label: t("setting.instance.monday") },
    ],
    [t],
  );

  const updatePartialSetting = (partial: Partial<InstanceSetting_GeneralSetting>) => {
    setInstanceGeneralSetting(
      create(InstanceSetting_GeneralSettingSchema, {
        ...instanceGeneralSetting,
        ...partial,
      }),
    );
  };

  const hasUnsavedChanges = !isEqual(instanceGeneralSetting, originalGeneralSetting);

  const handleSaveSettings = async () => {
    if (!hasUnsavedChanges) return;
    await saveInstanceSetting({
      key: InstanceSetting_Key.GENERAL,
      setting: create(InstanceSettingSchema, {
        name: buildInstanceSettingName(InstanceSetting_Key.GENERAL),
        value: {
          case: "generalSetting",
          value: create(InstanceSetting_GeneralSettingSchema, { ...instanceGeneralSetting, additionalScript: "" }),
        },
      }),
      errorContext: "Update general settings",
    });
  };

  return (
    <SettingSection title={t("setting.system.label")}>
      <SettingGroup title={t("common.basic")} description={t("setting.system.basic-description")}>
        <SettingList>
          <SettingListItem
            label={t("setting.system.server-name")}
            description={instanceGeneralSetting.customProfile?.title || DEFAULT_APP_NAME}
          >
            <Button variant="neutral" onClick={customizeDialog.open}>
              {t("common.edit")}
            </Button>
          </SettingListItem>

          <SettingListItem label={t("setting.instance.week-start-day")} description={t("setting.instance.week-start-day-description")}>
            <Select.Root
              value={instanceGeneralSetting.weekStartDayOffset.toString()}
              items={weekStartDayOptions}
              onValueChange={(value) => {
                updatePartialSetting({ weekStartDayOffset: parseInt(value ?? "") || 0 });
              }}
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

      <SettingGroup title={t("setting.system.custom-code-title")} description={t("setting.system.custom-code-description")} showSeparator>
        <SettingCodeEditor
          label={t("setting.system.additional-style")}
          description={t("setting.system.additional-style-description")}
          placeholder={t("setting.system.additional-style-placeholder")}
          value={instanceGeneralSetting.additionalStyle}
          onChange={(additionalStyle) => updatePartialSetting({ additionalStyle })}
        />
      </SettingGroup>

      <div className="w-full flex justify-end">
        <Button variant="primary" disabled={!hasUnsavedChanges} onClick={handleSaveSettings}>
          {t("common.save")}
        </Button>
      </div>

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
