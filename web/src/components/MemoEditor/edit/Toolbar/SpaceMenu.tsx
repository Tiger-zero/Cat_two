import { buttonClasses, Menu } from "@usememos/ui";
import { CheckIcon, ChevronDownIcon, PlusIcon } from "lucide-react";
import SpaceMark from "@/components/SpaceMark";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useSpaces } from "@/hooks/useSpaceQueries";
import { extractSpaceUidFromName, getDuplicateSpaceTitles } from "@/lib/space-display";
import { cn } from "@/lib/utils";
import type { Space_Icon } from "@/types/proto/api/space_service_pb";
import { useTranslate } from "@/utils/i18n";

interface SpaceMenuProps {
  space?: string;
  onChange: (space?: string) => void;
  disabled?: boolean;
}

/** Selects a Space for organizing a personal memo. */
export default function SpaceMenu({ space, onChange, disabled }: SpaceMenuProps) {
  const t = useTranslate();
  const user = useCurrentUser();
  const { data: spaces = [], isPending, isError, refetch } = useSpaces(user?.name);
  if (!space && !spaces.length && !isError) return null;

  const duplicates = getDuplicateSpaceTitles(spaces);
  const spaceLabel = (item: (typeof spaces)[number]) =>
    duplicates.has(item.title) ? `${item.title} (${extractSpaceUidFromName(item.name)})` : item.title;
  const selectedSpace = spaces.find((item) => item.name === space);
  const label = selectedSpace ? spaceLabel(selectedSpace) : space ? extractSpaceUidFromName(space) : t("space.add-to");
  // A memo in a Space the user has left can still be moved out of it.
  const choices: { name: string; label: string; icon?: Space_Icon }[] = [
    ...(space && !selectedSpace ? [{ name: space, label }] : []),
    ...spaces.map((item) => ({ name: item.name, label: spaceLabel(item), icon: item.icon })),
  ];

  return (
    <Menu.Root modal={false}>
      <Menu.Trigger
        className={cn(buttonClasses({ variant: "ghost", size: "md" }), "min-w-0 max-w-48 ps-(--mu-row-padding-icon)")}
        disabled={disabled}
        title={label}
      >
        {space ? <SpaceMark icon={selectedSpace?.icon} size="sm" /> : <PlusIcon className="mu-icon" />}
        <span className="min-w-0 truncate">{label}</span>
        <ChevronDownIcon className="mu-icon-sm" />
      </Menu.Trigger>
      <Menu.Popup align="start" className="w-56" aria-label={t("space.add-to")}>
        {choices.map((item) => {
          const selected = item.name === space;
          const itemLabel = selected ? t("space.remove-from", { space: item.label }) : item.label;
          return (
            <Menu.Item
              key={item.name}
              title={itemLabel}
              aria-label={itemLabel}
              onClick={() => onChange(selected ? undefined : item.name)}
              icon={<SpaceMark icon={item.icon} size="sm" />}
              shortcut={selected ? <CheckIcon className="mu-icon-sm" aria-hidden /> : undefined}
            >
              {item.label}
            </Menu.Item>
          );
        })}
        {isPending && <Menu.Item disabled>{t("space.loading")}</Menu.Item>}
        {isError && (
          <Menu.Item closeOnClick={false} onClick={() => void refetch()} shortcut={t("search.retry")}>
            <span className="text-mu-danger">{t("space.load-error")}</span>
          </Menu.Item>
        )}
      </Menu.Popup>
    </Menu.Root>
  );
}
