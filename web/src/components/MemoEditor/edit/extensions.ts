import {
  type Extension,
  fileHandler,
  lineTable,
  matchToken,
  type SuggestionAllowContext,
  scanTags,
  sourceRange,
  suggestion,
} from "@usememos/editor";
import { emoji } from "@usememos/editor/emoji";
import { parseMemoAst, visitWithSource } from "@/lib/memo-ast";
import type { EditorPosition } from "./types/editorController";

/** The gesture that handed files to the editor: a drop lands at a document position, a paste has none. */
export type EditorFileOrigin = { source: "paste" } | { source: "drop"; position: EditorPosition | null };

export interface EditorExtensionsOptions {
  /** Known tags, read when the popup asks for items. */
  getTags: () => string[];
  onFiles: (files: File[], origin: EditorFileOrigin) => void;
  /** Accessible name of the emoji popup. */
  emojiLabel: string;
}

/**
 * Ranks a candidate tag against the typed text (both already lower-cased).
 * Lower sorts first; `undefined` means the candidate does not match.
 *
 * Nested tags are paths, so a match inside the path counts: typing `mem` has to
 * reach `software/hosted/Memos`, which a full-path prefix test cannot do. The
 * tiers keep that from turning into noise — a segment start outranks an
 * incidental hit in the middle of a word.
 */
export const matchRank = (tag: string, typed: string): number | undefined => {
  const index = tag.indexOf(typed);
  if (index < 0) return undefined;
  if (index === 0) return 0; // Full-path prefix.
  if (tag[index - 1] === "/") return 1; // Path-segment start.
  return 2; // Anywhere else.
};

/** Known tags matching `query`, best first; incoming order is kept within a tier. */
export const rankTags = (tags: readonly string[], query: string): string[] => {
  const typed = query.toLowerCase();
  return tags
    .map((tag) => ({ tag, rank: matchRank(tag.toLowerCase(), typed) }))
    .filter((candidate): candidate is { tag: string; rank: number } => candidate.rank !== undefined)
    .sort((a, b) => a.rank - b.rank)
    .map(({ tag }) => tag);
};

/** The Markdown context at the insertion point, using the same parse as the memo renderer. */
export const sourceTagContext = (markdown: string, offset: number): "literal" | "heading-marker" | "prose" => {
  const lines = lineTable(markdown);
  let context: "literal" | "heading-marker" | "prose" = "prose";
  visitWithSource(parseMemoAst(markdown), (node, content) => {
    if (context === "literal") return;
    if (node.type === "codeBlock" && node.lines) {
      if (offset >= lines.starts[node.lines.start - 1] && offset <= lines.ends[node.lines.end - 1]) context = "literal";
    } else if ((node.type === "raw" || node.type === "htmlBlock") && node.source) {
      const { from, to } = sourceRange(lines, node.source, { from: 0, to: node.value.length });
      if (offset >= from && offset <= to) context = "literal";
    } else if (
      content &&
      (node.type === "code" || node.type === "link" || node.type === "image" || node.type === "html" || node.type === "token") &&
      node.pos
    ) {
      const { from, to } = sourceRange(lines, content, node.pos);
      if (offset > from && offset < to) context = "literal";
    } else if (node.type === "heading" && !node.setext && node.source?.lines[0]) {
      const { line, offset: contentOffset } = node.source.lines[0];
      const start = lines.starts[line - 1];
      if (offset >= start && offset < start + contentOffset) context = "heading-marker";
    }
  });
  return context;
};

/**
 * Where the tag popup may open. Not inside code or links, where `#` is literal,
 * and not for a bare `#` at the start of a line, which is more likely the start
 * of a heading. Ctrl+Space still opens it there.
 */
export const allowTagSuggestion = ({ excluded, explicit, query, textBefore, mode, editor, from }: SuggestionAllowContext): boolean => {
  if (excluded) {
    // A bare hash followed by the toolbar's separator looks like an ATX heading
    // until a tag is entered. An explicit request can complete that marker.
    return explicit && mode === "source" && query === "" && sourceTagContext(editor.getMarkdown(), from) === "heading-marker";
  }
  if (explicit) return true;
  return !(query === "" && /(^|\n)[ \t]*$/.test(textBefore));
};

/** What only the composer adds to the memo syntax MemoMarkdown already reads: autocomplete and file handling. */
export function buildEditorExtensions({ getTags, onFiles, emojiLabel }: EditorExtensionsOptions): Extension[] {
  return [
    // The popup opens exactly where tags are highlighted: the matcher is built
    // from the same scanner as tags(). A pick writes "#work/planning ".
    suggestion({
      name: "tags",
      trigger: ["#", "＃"],
      match: matchToken(scanTags),
      allow: allowTagSuggestion,
      items: ({ query }) => rankTags(getTags(), query).map((tag) => ({ id: tag, label: `#${tag}`, value: tag })),
    }),
    // ":tada" lists emoji by name; a pick writes the emoji itself. The list loads on first use.
    emoji({ label: emojiLabel }),
    // Pasted files join the attachment list (a paste carries no placement
    // gesture); dropped files land where they were dropped.
    fileHandler({
      onPaste: ({ files }) => onFiles(files, { source: "paste" }),
      onDrop: ({ files, position }) => onFiles(files, { source: "drop", position }),
    }),
  ];
}
