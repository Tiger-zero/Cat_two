import {
  type Block,
  commands,
  divider,
  type Editor,
  emptyTable,
  getBlock,
  inlineText,
  isCollapsed,
  isPlainTextBlock,
  isTextless,
  lineTable,
  serializeInline,
  serializeMarkdown,
  type Widget,
} from "@usememos/editor";
import { MEMO_MARKDOWN, MEMO_SYNTAX, parseMemoAst } from "@/lib/memo-ast";
import { collectTags, parseMemoMarkdown } from "@/lib/memo-markdown";
import { isAllowedLinkUrl } from "@/lib/url-policy";
import { isCompleteTagValue } from "@/utils/tag-grammar";
import type { EditorController, EditorFormat } from "./types/editorController";
import { renderUploadAnchor } from "./uploadAnchors";

/** The live editor, or null before it mounts and after it is destroyed. */
type EditorGetter = () => Editor | null;

const live = (getEditor: EditorGetter): Editor | null => {
  const editor = getEditor();
  return editor && !editor.isDestroyed ? editor : null;
};

/**
 * The memo's blocks. Source mode holds the whole memo as one block of Markdown
 * text, so the blocks are read from that text instead.
 */
const memoBlocks = (editor: Editor): readonly Block[] =>
  editor.mode === "source" ? parseMemoMarkdown(editor.getMarkdown()).blocks : editor.state.doc.blocks;

/** An empty paragraph, which inserted blocks take the place of rather than follow. */
const isEmptyParagraph = (block: Block): boolean => block.type === "paragraph" && inlineText(block.content) === "";

const insertSourceBlock = (editor: Editor, block: Block): boolean => {
  const source = editor.getMarkdown();
  const { focus } = editor.getSelection();
  const { ends } = lineTable(source);
  const line = ends.findIndex((end) => focus.offset <= end) + 1;
  // Insert after the current block to keep existing tables, fences and selected text intact.
  const current = parseMemoAst(source).lines.find(({ start, end }) => start <= line && line <= end);
  const offset = ends[(current?.end ?? line) - 1] ?? source.length;
  const before = source.slice(0, offset);
  const after = source.slice(offset);
  const beforeBreaks = before.slice(-4).replace(/\r\n?/g, "\n");
  const afterBreaks = after.slice(0, 4).replace(/\r\n?/g, "\n");
  const prefix = !before || beforeBreaks.endsWith("\n\n") ? "" : beforeBreaks.endsWith("\n") ? "\n" : "\n\n";
  const suffix = !after || afterBreaks.startsWith("\n\n") ? "" : afterBreaks.startsWith("\n") ? "\n" : "\n\n";
  const markdown = prefix + serializeMarkdown({ blocks: [block] }).trimEnd() + suffix;
  // An unclosed code fence or an HTML block can swallow Markdown after a blank line.
  const insertedLine = lineTable(before + prefix).starts.length;
  const result = parseMemoAst(before + markdown + after);
  const type = block.type === "table" ? "table" : "thematicBreak";
  if (!result.children.some((node, index) => node.type === type && result.lines[index]?.start === insertedLine)) return false;
  return editor.commands.insertMarkdown(markdown, { at: { ...focus, offset } });
};

const formatCommands: Record<EditorFormat, (editor: Editor) => boolean> = {
  bold: (editor) => editor.commands.toggleBold(),
  italic: (editor) => editor.commands.toggleItalic(),
  strike: (editor) => editor.commands.toggleStrike(),
  inlineCode: (editor) => editor.commands.toggleCode(),
  paragraph: (editor) => editor.commands.setParagraph(),
  heading1: (editor) => editor.commands.toggleHeading({ level: 1 }),
  heading2: (editor) => editor.commands.toggleHeading({ level: 2 }),
  heading3: (editor) => editor.commands.toggleHeading({ level: 3 }),
  blockquote: (editor) => editor.commands.toggleBlockquote(),
  bulletList: (editor) => editor.commands.toggleBulletList(),
  orderedList: (editor) => editor.commands.toggleOrderedList(),
  taskList: (editor) => editor.commands.toggleTaskList(),
  codeBlock: (editor) => editor.commands.toggleCodeBlock(),
  table: (editor) => (editor.mode === "source" ? insertSourceBlock(editor, emptyTable()) : editor.commands.insertTable()),
  divider: (editor) => (editor.mode === "source" ? insertSourceBlock(editor, divider()) : editor.commands.insertDivider()),
};

const linkMarkdown = (label: string, href: string): string =>
  // An empty title keeps explicit brackets even when the label equals the URL.
  // Encode pipes so the link can also be inserted in a Markdown table cell.
  serializeInline([{ text: label, link: href, linkTitle: "", marks: [] }], { ...MEMO_MARKDOWN, syntax: MEMO_SYNTAX }).replace(
    /\|/g,
    "&#124;",
  );

const applyLink = (editor: Editor, href: string): boolean => {
  const selection = editor.getSelection();
  const collapsed = isCollapsed(selection);
  if (editor.mode === "rich") {
    if (!collapsed) return editor.commands.setLink({ href });
    const block = getBlock(editor.state.doc, selection.focus.blockId);
    return !isPlainTextBlock(block) && !isTextless(block) && editor.commands.insertMarkdown(linkMarkdown(href, href));
  }

  const { anchor, focus } = selection;
  const start = Math.min(anchor.offset, focus.offset);
  const end = Math.max(anchor.offset, focus.offset);
  const label = collapsed ? href : editor.getMarkdown().slice(start, end);
  return editor.commands.insertMarkdown(linkMarkdown(label, href));
};

/**
 * EditorController over a @usememos/editor instance. Every method reads the
 * live editor when called, so the controller can be handed out before the
 * editor mounts, and calls after it is destroyed do nothing. The editor does
 * not emit a change for a replaced document, so `onReplace` reports it.
 */
export function createController(getEditor: EditorGetter, onReplace: (markdown: string) => void): EditorController {
  const anchors = new Map<string, Widget>();
  const currentTags = (editor: Editor) => collectTags(memoBlocks(editor));

  const takeAnchor = (id: string): Widget | undefined => {
    const widget = anchors.get(id);
    anchors.delete(id);
    return widget && !widget.isRemoved ? widget : undefined;
  };

  return {
    focus: () => live(getEditor)?.focus(),
    hasFocus: () => live(getEditor)?.hasFocus() ?? false,
    isEmpty: () => (live(getEditor)?.getMarkdown() ?? "").trim() === "",
    getMarkdown: () => getEditor()?.getMarkdown() ?? "",
    setMarkdown: (markdown) => {
      const editor = live(getEditor);
      if (!editor) return;
      editor.setMarkdown(markdown);
      onReplace(markdown);
    },
    insertMarkdown: (markdown) => {
      const editor = live(getEditor);
      if (!editor || !markdown) return;
      // Insert at the caret without consuming the selection: callers live outside
      // the editor (attachment list, audio recorder), and replacing the range
      // would delete text the user merely had highlighted. An empty paragraph at
      // the caret is replaced, as a paste would.
      editor.commands.insertMarkdown(markdown, { at: editor.getSelection().focus, block: true });
      editor.commands.focus();
      editor.commands.scrollIntoView();
    },
    getTags: () => {
      const editor = live(getEditor);
      return editor ? currentTags(editor) : [];
    },
    hasChecklist: () => {
      const editor = live(getEditor);
      return editor ? memoBlocks(editor).some((block) => block.type === "listItem" && block.listType === "task") : false;
    },
    insertTag: (tag) => {
      const editor = live(getEditor);
      if (!editor || !isCompleteTagValue(tag) || currentTags(editor).includes(tag)) return false;
      // As a word of its own at the caret, keeping highlighted text and the Markdown
      // context, including code; refused while an input method is composing.
      if (!editor.commands.insertToken(`#${tag}`)) return false;
      editor.commands.focus();
      editor.commands.scrollIntoView();
      return true;
    },
    createUploadAnchor: (descriptor, position) => {
      const editor = live(getEditor);
      if (!editor) return;
      takeAnchor(descriptor.id)?.remove();
      const at = position ?? editor.getSelection().focus;
      anchors.set(descriptor.id, editor.addWidget({ at, render: renderUploadAnchor(descriptor) }));
    },
    updateUploadAnchor: (descriptor) => {
      const widget = anchors.get(descriptor.id);
      if (widget && !widget.isRemoved) widget.update(renderUploadAnchor(descriptor));
    },
    resolveUploadAnchor: (id, markdown) => {
      const editor = live(getEditor);
      const widget = takeAnchor(id);
      if (!editor || !widget) return;
      if (markdown) {
        // The anchor sits after its block; an empty block gives its place to the content.
        const block = editor.state.doc.blocks.find((candidate) => candidate.id === widget.blockId);
        const at = block && isEmptyParagraph(block) ? { blockId: block.id, offset: 0 } : widget;
        editor.commands.insertMarkdown(markdown, { at, block: true });
        editor.commands.focus();
        editor.commands.scrollIntoView();
      }
      widget.remove();
    },
    cancelUploadAnchor: (id) => takeAnchor(id)?.remove(),
    getSelection: () => live(getEditor)?.selectionToJSON(),
    setSelection: (selection) => {
      const editor = live(getEditor);
      if (!editor) return;
      editor.setSelection(selection);
      editor.commands.scrollIntoView();
    },
    selectAll: () => live(getEditor)?.commands.selectAll(),
    toggleFormat: (format) => {
      const editor = live(getEditor);
      if (!editor || editor.isComposing) return false;
      const changed = formatCommands[format](editor);
      editor.commands.focus();
      if (changed) editor.commands.scrollIntoView();
      return changed;
    },
    setLink: (href) => {
      const editor = live(getEditor);
      if (!editor || editor.isComposing) return false;
      const url = href.trim();
      if (!url || /[\r\n]/.test(url) || !isAllowedLinkUrl(url) || !applyLink(editor, url)) return false;
      editor.commands.focus();
      editor.commands.scrollIntoView();
      return true;
    },
    getActiveFormats: () => {
      const editor = live(getEditor);
      if (!editor || editor.mode !== "rich") return [];

      const active: EditorFormat[] = [];
      if (editor.isActive("bold")) active.push("bold");
      if (editor.isActive("italic")) active.push("italic");
      // Block commands toggle off only when every convertible block already has the format.
      const blocks = commands.convertibleBlocks(editor.state);
      const first = blocks[0];
      if (
        first?.type === "listItem" &&
        first.listType !== "task" &&
        blocks.every((block) => block.type === "listItem" && block.listType === first.listType)
      ) {
        active.push(first.listType === "bullet" ? "bulletList" : "orderedList");
      }
      if (first?.type === "code" && blocks.every((block) => block.type === "code")) active.push("codeBlock");
      return active;
    },
    subscribeToFormatting: (listener) => live(getEditor)?.on("selectionChange", listener) ?? (() => {}),
    toggleTaskList: () => {
      const editor = live(getEditor);
      return !!editor && !editor.isComposing && formatCommands.taskList(editor);
    },
  };
}
