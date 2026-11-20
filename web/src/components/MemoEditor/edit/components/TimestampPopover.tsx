import { Field, Input, Popover } from "@usememos/ui";
import { type FC, useRef, useState } from "react";
import { MEMO_TIME_CONTROL_CLASSES } from "@/components/MemoView/constants";
import i18n from "@/i18n";
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

/** "Now" in the viewer's language, the word the memo card shows for a memo written this moment. */
function nowLabel(): string {
  const word = new Intl.RelativeTimeFormat(i18n.language, { numeric: "auto" }).format(0, "second");
  return word.charAt(0).toLocaleUpperCase(i18n.language) + word.slice(1);
}

/** The memo's time as a control; a memo being written that has no time yet reads "Now". */
export const TimestampPopover: FC = () => {
  const t = useTranslate();
  const { actions, dispatch } = useEditorContext();
  const createTime = useEditorSelector((s) => s.timestamps.createTime);
  const updateTime = useEditorSelector((s) => s.timestamps.updateTime);

  if (!createTime) return <span className="shrink-0 whitespace-nowrap mu-caption text-mu-text-secondary select-none">{nowLabel()}</span>;

  return (
    <Popover.Root>
      <Popover.Trigger render={<button type="button" className={MEMO_TIME_CONTROL_CLASSES} />}>{formatDate(createTime)}</Popover.Trigger>
      <Popover.Popup align="start" className="w-auto">
        <TimestampInput
          label={t("common.created-at")}
          date={createTime}
          onChange={(d) => dispatch(actions.setTimestamps({ createTime: d }))}
        />
        <TimestampInput
          label={t("common.last-updated-at")}
          date={updateTime}
          onChange={(d) => dispatch(actions.setTimestamps({ updateTime: d }))}
        />
      </Popover.Popup>
    </Popover.Root>
  );
};
