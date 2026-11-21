import { Button, Menu, Tooltip } from "@usememos/ui";
import {
  BoldIcon,
  CodeXmlIcon,
  EllipsisIcon,
  Heading1Icon,
  Heading2Icon,
  Heading3Icon,
  ItalicIcon,
  LinkIcon,
  ListIcon,
  ListOrderedIcon,
  ListTodoIcon,
  MinusIcon,
  PilcrowIcon,
  QuoteIcon,
  SquareCodeIcon,
  StrikethroughIcon,
  Table2Icon,
} from "lucide-react";
import { type RefObject, useEffect, useRef, useState } from "react";
import { showErrorToast } from "@/lib/toast";
import { useTranslate } from "@/utils/i18n";
import { primaryModifierGlyph } from "@/utils/platform";
import type { EditorController, EditorFormat, EditorSelection } from "../types/editorController";
import WebLinkDialog from "./WebLinkDialog";

const formats = [
  { format: "bold", label: "editor.format.bold", icon: BoldIcon, shortcut: "B" },
  { format: "italic", label: "editor.format.italic", icon: ItalicIcon, shortcut: "I" },
  { format: "bulletList", label: "editor.format.bullet-list", icon: ListIcon, shortcut: undefined },
  { format: "orderedList", label: "editor.format.ordered-list", icon: ListOrderedIcon, shortcut: undefined },
  { format: "codeBlock", label: "editor.format.code-block", icon: SquareCodeIcon, shortcut: undefined },
] as const;

const moreFormats = [
  { format: "paragraph", label: "editor.format.paragraph", icon: PilcrowIcon },
  { format: "heading1", label: "editor.format.heading-1", icon: Heading1Icon },
  { format: "heading2", label: "editor.format.heading-2", icon: Heading2Icon },
  { format: "heading3", label: "editor.format.heading-3", icon: Heading3Icon },
  { format: "strike", label: "editor.format.strikethrough", icon: StrikethroughIcon },
  { format: "inlineCode", label: "editor.format.inline-code", icon: CodeXmlIcon },
  { format: "blockquote", label: "editor.blocks.quote", icon: QuoteIcon },
  { format: "taskList", label: "editor.format.task-list", icon: ListTodoIcon },
  { format: "table", label: "editor.blocks.table", icon: Table2Icon },
  { format: "divider", label: "editor.blocks.divider", icon: MinusIcon },
] as const;

interface Props {
  editorRef: RefObject<EditorController | null>;
  disabled: boolean;
  sourceMode: boolean;
}

export default function FormattingControls({ editorRef, disabled, sourceMode }: Props) {
  const t = useTranslate();
  const [activeFormats, setActiveFormats] = useState<EditorFormat[]>([]);
  const [linkTarget, setLinkTarget] = useState<{ selection: EditorSelection | undefined }>();
  const moreButtonRef = useRef<HTMLButtonElement>(null);
  const menuSelection = useRef<EditorSelection>(undefined);
  const afterMenuClose = useRef<"focusEditor" | "webLink">(undefined);
  const keepCurrentFocus = useRef(false);

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

  const applyMenuFormat = (format: EditorFormat) => {
    keepCurrentFocus.current = true;
    afterMenuClose.current = "focusEditor";
    const editor = editorRef.current;
    if (menuSelection.current) editor?.setSelection(menuSelection.current);
    editor?.toggleFormat(format);
  };

  const finishMenuClose = (open: boolean) => {
    if (open) return;
    const action = afterMenuClose.current;
    afterMenuClose.current = undefined;
    if (action === "webLink") {
      setLinkTarget({ selection: menuSelection.current });
    } else if (action === "focusEditor") {
      // The menu's closing focus management can blur the editor after the command.
      editorRef.current?.focus();
    }
  };

  const applyLink = (href: string) => {
    const editor = editorRef.current;
    if (linkTarget?.selection) editor?.setSelection(linkTarget.selection);
    if (!editor?.setLink(href)) showErrorToast(t("editor.format.link-not-applied"));
  };

  return (
    <div role="group" aria-label={t("editor.format.label")} className="flex min-w-0 flex-wrap items-center gap-0 sm:gap-0.5">
      {formats.map(({ format, label, icon: Icon, shortcut }) => {
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
                  aria-keyshortcuts={shortcut ? `Meta+${shortcut} Control+${shortcut}` : undefined}
                  disabled={disabled}
                  icon={<Icon />}
                  // Keep the live text selection while clicking a control outside the editable surface.
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => editorRef.current?.toggleFormat(format)}
                />
              }
            />
            <Tooltip.Popup shortcut={shortcut ? `${primaryModifierGlyph()} ${shortcut}` : undefined}>{t(label)}</Tooltip.Popup>
          </Tooltip.Root>
        );
      })}
      <Menu.Root
        modal={false}
        onOpenChange={(open, details) => {
          if (open) {
            keepCurrentFocus.current = false;
            afterMenuClose.current = undefined;
            menuSelection.current = editorRef.current?.getSelection();
          } else if (details.reason === "outside-press" || details.reason === "focus-out") {
            keepCurrentFocus.current = true;
          }
        }}
        onOpenChangeComplete={finishMenuClose}
      >
        <Tooltip.Root>
          <Tooltip.Trigger
            render={
              <Menu.Trigger
                render={
                  <Button
                    ref={moreButtonRef}
                    type="button"
                    variant="ghost"
                    aria-label={t("editor.format.more")}
                    disabled={disabled}
                    icon={<EllipsisIcon />}
                    onMouseDown={(event) => event.preventDefault()}
                  />
                }
              />
            }
          />
          <Tooltip.Popup>{t("editor.format.more")}</Tooltip.Popup>
        </Tooltip.Root>
        {/* Keep focus when it moves to the editor, link dialog or a control outside the menu. */}
        <Menu.Popup align="start" side="top" finalFocus={() => !keepCurrentFocus.current}>
          {moreFormats.map(({ format, label, icon: Icon }) => (
            <Menu.Item key={format} disabled={disabled} icon={<Icon />} onClick={() => applyMenuFormat(format)}>
              {t(label)}
            </Menu.Item>
          ))}
          <Menu.Separator />
          <Menu.Item
            disabled={disabled}
            icon={<LinkIcon />}
            onClick={() => {
              keepCurrentFocus.current = true;
              afterMenuClose.current = "webLink";
            }}
          >
            {t("editor.format.web-link")}
          </Menu.Item>
        </Menu.Popup>
      </Menu.Root>
      {linkTarget && (
        <WebLinkDialog
          triggerRef={moreButtonRef}
          onClose={(submitted) => {
            if (submitted) editorRef.current?.focus();
            setLinkTarget(undefined);
          }}
          onApply={applyLink}
        />
      )}
    </div>
  );
}
