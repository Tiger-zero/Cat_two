import { Field, Input, Popover } from "@usememos/ui";
import { type FC, useRef, useState } from "react";
import { MEMO_TIME_CONTROL_CLASSES } from "@/components/MemoView/constants";
import { recordMemoEditingActivity } from "@/lib/memo-editing-activity";
import { APP_DATE_TIME_FORMAT, inAppTimeZone, parseAppDateTime } from "@/lib/time-zone";
import { useTranslate } from "@/utils/i18n";
import { useEditorContext, useEditorSelector } from "../state";

const formatDate = (date: Date): string => inAppTimeZone(date).format(APP_DATE_TIME_FORMAT);

const TimestampInput: FC<{
  label: string;
  date: Date | undefined;
  onChange: (date: Date) => void;
}> = ({ label, date, onChange }) => {
  const initialValue = useRef(date ? formatDate(date) : "");
  const [value, setValue] = useState(initialValue.current);
  const [invalid, setInvalid] = useState(false);

  const handleChange = (nextValue: string) => {
    setValue(nextValue);
    const parsed = parseAppDateTime(nextValue);
    setInvalid(false);
    if (parsed) {
      // Closing the popover can unmount the input without firing blur.
      // Keep valid edits in the editor store as soon as they are entered.
      onChange(parsed);
    }
  };

  return (
    <Field.Root invalid={invalid}>
      <Field.Label>
        {label}
        {value !== initialValue.current && <span className="text-mu-primary ml-0.5">*</span>}
      </Field.Label>
      <Input
        type="text"
        className="font-mono"
        aria-invalid={invalid}
        placeholder={APP_DATE_TIME_FORMAT}
        value={value}
        onChange={(e) => handleChange(e.target.value)}
        onBlur={() => setInvalid(!parseAppDateTime(value))}
      />
    </Field.Root>
  );
};

export const TimestampPopover: FC = () => {
  const t = useTranslate();
  const { actions, dispatch, getState } = useEditorContext();
  const saving = useEditorSelector((s) => s.ui.isLoading.saving);
  const createTime = useEditorSelector((s) => s.timestamps.createTime);
  const updateTime = useEditorSelector((s) => s.timestamps.updateTime);

  if (!createTime) return null;

  const setUpdateTime = (date: Date) => {
    const current = getState();
    if (current.ui.isLoading.saving || current.timestamps.updateTime?.getTime() === date.getTime()) return;
    dispatch(actions.setTimestamps({ updateTime: date }));
    recordMemoEditingActivity();
  };

  return (
    <Popover.Root>
      <Popover.Trigger disabled={saving} render={<button type="button" className={MEMO_TIME_CONTROL_CLASSES} />}>
        {formatDate(createTime)}
      </Popover.Trigger>
      <Popover.Popup align="start" className="w-auto">
        <div>
          <div>{t("common.created-at")}</div>
          <time dateTime={createTime.toISOString()} className="font-mono">
            {formatDate(createTime)}
          </time>
        </div>
        <TimestampInput label={t("common.last-updated-at")} date={updateTime} onChange={setUpdateTime} />
      </Popover.Popup>
    </Popover.Root>
  );
};
