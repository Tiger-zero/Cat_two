import { Button, Tooltip } from "@usememos/ui";
import { BoldIcon, ListIcon, ListOrderedIcon, SquareCodeIcon } from "lucide-react";
import { type RefObject, useEffect, useState } from "react";
import { useTranslate } from "@/utils/i18n";
import { primaryModifierGlyph } from "@/utils/platform";
import type { EditorController, EditorFormat } from "../types/editorController";

const formats = [
  { format: "bold", label: "editor.format.bold", icon: BoldIcon },
  { format: "bulletList", label: "editor.format.bullet-list", icon: ListIcon },
  { format: "orderedList", label: "editor.format.ordered-list", icon: ListOrderedIcon },
  { format: "codeBlock", label: "editor.format.code-block", icon: SquareCodeIcon },
] as const;

interface Props {
  editorRef: RefObject<EditorController | null>;
  disabled: boolean;
  sourceMode: boolean;
}

export default function FormattingControls({ editorRef, disabled, sourceMode }: Props) {
  const t = useTranslate();
  const [activeFormats, setActiveFormats] = useState<EditorFormat[]>([]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor || sourceMode) return;
    const refresh = () => {
      const next = editor.getActiveFormats();
      setActiveFormats((current) => (current.length === next.length && current.every((format, i) => format === next[i]) ? current : next));
    };
    refresh();
    return editor.subscribeToFormatting(refresh);
  }, [editorRef, sourceMode]);

  return (
    <div role="group" aria-label={t("editor.format.label")} className="flex min-w-0 flex-wrap items-center gap-0.5">
      {formats.map(({ format, label, icon: Icon }) => {
        const active = !sourceMode && activeFormats.includes(format);
        return (
          <Tooltip.Root key={format}>
            <Tooltip.Trigger
              render={
                <Button
                  type="button"
                  variant={active ? "neutral" : "ghost"}
                  aria-label={t(label)}
                  aria-pressed={sourceMode ? undefined : active}
                  aria-keyshortcuts={format === "bold" ? "Meta+B Control+B" : undefined}
                  disabled={disabled}
                  icon={<Icon />}
                  // Keep the live text selection while clicking a control outside the editable surface.
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => editorRef.current?.toggleFormat(format)}
                />
              }
            />
            <Tooltip.Popup shortcut={format === "bold" ? `${primaryModifierGlyph()} B` : undefined}>{t(label)}</Tooltip.Popup>
          </Tooltip.Root>
        );
      })}
    </div>
  );
}
