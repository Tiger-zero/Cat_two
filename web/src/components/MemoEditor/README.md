# MemoEditor

## Overview

`MemoEditor` is the one way a memo is shown, edited or composed. It is memo-aware: it takes a memo's resource name, or a seed for a new memo or comment, and a mode. In view mode it reads the memo from whatever query already holds it and shows it read-only. In edit mode it owns a draft store and renders the editing session over it. The parent owns the mode and hears back through `onConfirm` and `onCancel`.

Both modes show the memo through the same binding of [`@usememos/editor`](https://github.com/usememos/editor), a block editor whose storage format is standard Markdown, so a memo looks the same while editing and after saving. Blocks the user did not edit are saved byte for byte; edited blocks are written in a normalized style.

```tsx
<MemoEditor name="memos/abc" />                                     // view; the owner can tick tasks
<MemoEditor mode="edit" name="memos/abc" onConfirm onCancel />      // edit in place
<MemoEditor draft={{ parent: "memos/abc" }} onConfirm onCancel />   // new comment
<MemoEditor draft={{ scope: "home", createTime }} />                // new memo
```

## Layers

```
MemoEditor                 memo level: name | draft seed, mode, data, draft store, mutations
├── MemoContentView        the read-only surface: navigation, anchors, link cards, search highlight
│   └── MemoMarkdown       the editor binding: dialect, theme, labels, URL policy, memo syntax, decorations
└── EditSession            the editing surface: chrome, uploads, audio, suggestions, save
    └── Editor             MemoMarkdown editable, with the composer's extensions, behind EditorController
```

- **`MemoMarkdown.tsx`** binds the editor to memos once: the memo Markdown dialect, the theme in the app's CSS variables, translated labels, the image and link URL policy, the memo syntax extensions (math, footnotes, tags, mentions, highlighted code, Mermaid diagrams drawn by Pele), raw HTML through the editor's `sanitizeHtml` option (`memoSanitizeHtml`: DOMPurify with memos' allowlist in `lib/html-sanitize-schema.ts`, trusted iframe hosts, and link and image defaults; loaded on first use) and the stylesheet. `useMemoDecorations` is the one flow for tag colors and resolved mentions, with one redecorate effect. Everything a surface adds sits around this component; nothing in memos styles the editor's own classes. The tag and mention classes memos hands the editor apply in source mode too, so the parts that draw an inline tint take the `rich:` variant (index.css), which is everything outside the editor's `.mbe-source-mode` root; source mode shows a tag as text in its color.
- **`view/`** is the read-only surface over `MemoMarkdown`: tags and mentions navigate, in-memo anchors scroll, a paragraph that is one bare link becomes a preview card through the editor's block view (`linkCards.ts`), the page's search matches are painted, and heading ids match the outline. Tag and mention clicks arrive through the editor's `onClick` callbacks. Read-only tags with a click handler are focusable buttons; Enter activates on press and Space on release through the same click callback. Editable tags remain text. Nothing in memos matches the editor's class names or mutates its DOM. Previews that have Markdown but no memo query (the link-memo search, the share image) use `MemoContentView` directly.
- **`edit/`** is the editing surface: `EditSession` renders the chrome (timestamp, attachments, suggestions, toolbar, focus mode, audio recording) over the draft store `MemoEditor` provides, persists the draft while open, and saves through the memo mutations. `Editor.tsx` is the composer's editor: `MemoMarkdown` editable with the composer's own extensions (`#tag` and `:emoji` autocomplete, file paste and drop), behind the `EditorController` contract in `controller.ts`.

Regular memo content uses 14px text on 25.2px lines, with 0.1px tracking and 12px between blocks. The reader and editor share Barlow and the same Chinese font fallback stack. The editor keeps list rows compact. Rich-text code blocks use the full `well` fill so they remain distinct from the surrounding surface in Dark. Markdown source mode stays transparent. Task items use `control-edge` for their checkbox border and darken Dark's pale accent behind the editor's white checkmark. Callouts use the app's accent, success, warning and danger colors. Custom tag tints bound lightness and sRGB channels so black and white remain visible across themes without coloring the text.

Code blocks use lazy-loaded `@pierre/highlights` tokens as editor decorations, without an HTML conversion. GitHub Light and GitHub Dark Dimmed colors follow the document's color scheme; ordinary text and structural punctuation keep the editor's text color. Unsupported languages, unavailable WebAssembly, or browsers without `light-dark()` colors leave the code as plain text. Highlighting never changes the saved Markdown.

## Data

- **Reads are by name.** `useMemoByName` (`hooks/useMemoQueries.ts`) returns the freshest copy of the memo any query holds (a list, a comment page, the calendar month, a detail, a shared memo) and fetches the detail only when nothing holds it. A card in a list therefore costs no request beyond the list's, follows the list's refetches, and never refetches on its own.
- **Writes are mutations.** A ticked task, a save and a new memo go through `useUpdateMemo` and `useCreateMemo`, which patch and refresh every query holding the memo, so the saved text shows wherever the memo is on the page as soon as the server answers.
- **Drafts stay in the current browser.** `edit/draftKey.ts` scopes memo edits, replies and new notes by account and subject. `initialEditorState` restores text and saved attachment references from localStorage. A recovered edit must match the memo's saved revision. `useStagedDraftFiles` restores files awaiting upload from IndexedDB and recreates their preview URLs. Writes and removal compare the last observed snapshot to detect changes from another tab. Successful saves and cancellation of an existing edit clear the session's draft when its snapshot still matches.
- **Home restores drafts in priority order.** Its own draft comes first. On a fresh mount with an empty Home draft, the composer can open a draft from the former sidebar editor for the current account. The saved text and pending files for that draft use their original key.
- **Calendar defaults follow the selected Shanghai day.** Manual create and update times survive filter changes and refreshes for that day. Changing or clearing the date filter resets the defaults for the new selection.

## The editing session

### State

A reducer (`edit/state/reducer.ts`) drives an external store; consumers subscribe to one slice via `useEditorSelector`, so typing re-renders only what reads `content`. `state.content` is the document as Markdown and the single source of truth. UI flags (focus mode, loading, pending inline images), metadata (Space, attachments, relations), timestamps, local files awaiting upload, the shared order of saved attachments and new files (`attachmentOrder`) and accepted suggestions live beside it.

### The editor contract

`edit/types/editorController.ts` defines `EditorController`: document access (`getMarkdown`, `setMarkdown`, `insertMarkdown`), selection and focus, formatting and checklist commands, the upload-anchor group, `getTags`, `hasChecklist`, `startTag` and `insertTag`. The chrome uses this interface. `edit/controller.ts` implements it over the live editor. Calls before mount or after destroy do nothing.

### Formatting and source mode

The toolbar beneath the editor offers tag insertion, bold, italic, bulleted list, numbered list and code block. **Insert tag** puts `#` at the caret in prose, spaces it from neighbouring text, keeps selected text and opens existing tag suggestions. Continue typing to write a new tag. The button works in rich text and Markdown mode. **More formatting** offers paragraph and headings, strikethrough, inline code, blockquote, tasks, tables, dividers and links. The formatting controls apply to the current selection or block and return focus to the editor. Rich text also shows active formatting and provides the editor's floating selection toolbar, slash menu, block handle and link card.

Markdown mode opens from the ＋ menu and shows the entire memo as syntax-highlighted Markdown in the body font. `useSourceMode` (`@/hooks/useSourceMode`) shares the account's `EDITOR` user setting (`source_mode`) with Settings → Editor. A switch applies immediately, preserves the memo text and reverts if saving the preference fails. In Markdown mode the composer controls and formatting shortcuts edit the Markdown text. The `#tag` and `:emoji` popups work in both modes, and the checklist chip uses the same `toggleTaskList` command to toggle the caret's line. The controller parses the memo to read its tags and checklist items.

Markdown submitted through the API or MCP uses the same rendering as notes written in the editor. The memo renderer supports ATX headings, tables, task lists, blockquotes, code fences, links, GitHub-style alerts, footnotes and math. Both the editor and reader use the same Markdown dialect and URL policy.

### Saving

`useMemoSave` validates the draft, uploads local files, and hands the save to `memoService.save` with the operations it runs: the server's current copy (fetched into the detail cache to diff against), `useUpdateMemo` and `useCreateMemo`. A successful save resets the session and notifies the host immediately; editing and comment hosts close without a timed confirmation, and the in-place composer is ready for the next memo. The commit button stays mounted with its original label through validation and saving; its spinner and any drawing-save error use the same leading slot. Uploads that landed before a failed save stay on the draft as attachments so a retry does not upload them again. The save keeps the draft's attachment order (`edit/utils/attachmentOrder.ts`): new files land where they were dragged among the saved ones, not after them.

The editor stores the creation ID and submitted snapshot before sending a new note or comment. A retry can replay the same request after a reload, including the original calendar timestamp. When browser storage is unavailable, the editor keeps that ID in memory for retries in the same tab. An edit carries the memo's server revision. If the server rejects a stale revision, the draft and pending files can be opened again through **Review saved draft**, which fetches the current memo before restoring the draft for review.

### Attachments and links

The draft's attachments take the saved memo's shape. Images, videos and drawings form a gallery at the composer's 10rem height (`resolveEditorGalleryLayout`). Audio and files appear in a list with the draft's links. Saved attachments and new files share one order, a list of keys that stays empty until the first reorder. A key is an attachment's name, a new file's preview URL, or the name an edited drawing replaces. Tiles reorder by drag (a long press on touch) or Alt + arrow keys. Rows reorder by dragging their grip or using arrow keys on the focused grip. `useDragReorder` handles pointer events. Tile actions appear in the corner on hover or in the More menu on touch. "New" and "Edited" labels mark files waiting to be saved. While a memo is edited in place, the memos that link to it stay listed and read-only.

### Drawings

The contract and acceptance cases are in
[the drawing annotation and recovery design](../../../../.agents/skills/cat-two-development/references/design/drawing-annotation-and-recovery.md).
Canvas is pinned to `26.11.3`; its editable PNG remains an ordinary image outside the drawing UI.

The insert menu starts a drawing. Ordinary local and instance-hosted images offer **Markup**; editable PNGs offer
**Edit markup**. Annotation loads the original bytes, opens a locked background with a clean undo baseline, and starts with red
ink. Applying it keeps the original and inserts a new `.canvas.png` beside it. Saved image previews provide the same annotation
flow, with **Save copy** binding directly to that memo. External images and motion-photo groups are excluded.

The sheet has one short commit action: **Add** for new work, **Save** for edits, and **Save copy** for direct annotation or a copy
required by ownership. The editor's own save uploads pending files. An owned saved drawing uses `LocalFile.replaces`, preserving
its attachment name and links. Input is frozen before export until the host accepts or rejects it. Retries retain acknowledged
uploads in memory and finish the remaining bind or timestamp update; partial failures show an exception icon and tooltip.

This first release has no Memos markup-draft UI or persistence. **Close** confirms discarding unsaved changes; it does not retain a
recoverable markup. There is no Continue entry, no markup-dependent memo-save blocker, and no permanent More/Download/Discard menu.
Normal states add no copy or badges to the canvas. Viewing and editing share one dialog and Canvas instance, retaining the visible content, pan and zoom across mode changes. Both modes keep a ghost X at the top left and the current action at the top right, without bordered action containers or a visible mode heading. A live zoom percentage opens the same four-action zoom menu in both modes (zoom in, zoom out, fit, reset to 100%). The trigger stays at the bottom left, above the phone tools bar and clear of the safe area. Editing adds the tools bar at the top center on desktop and at the bottom on phones. Available phone Undo/Redo actions follow X as quiet ghost icons; unavailable history actions are omitted. Returning to view focuses Edit; discarding restores the accepted scene.

Canvas `26.11.3` provides checkpoints for a future recovery integration. Drawings already added to a draft can be recovered from
IndexedDB like other staged files. The server preserves editable PNGs unchanged (`server/fileserver/drawing_png_test.go`).
Browser regressions live in `tests/browser/`: run `pnpm test:drawings` after `pnpm exec playwright install chromium`. They verify
image preparation, original bytes/order, active Chinese text at export, desktop/360px controls, and view/edit content, viewport, focus and discard behavior.

### Suggestions

Hosts of a new top-level memo pass `useMemoSuggestions()` into `suggestions`; edits and comments show none. `EditorSuggestions` hides until the draft has text, drops suggestions already satisfied or accepted, and shows at most three. Acceptance history lives in the store so Undo does not immediately resuggest, and clears with the draft.

### Presentation in a card or inline

- **In a card** (`frameless`): the card is the frame, so the session has no background, border or padding of its own outside focus mode. `MemoView` flips its body into the session by name and keeps its header, frame and reactions mounted.
- **Inline** (Home, panels): the session brings its own frame. The ＋ menu offers focus mode, which expands it over the page; the exit button minimizes it back.

Focus mode is a sheet narrower than the page (`max-w-3xl`): its header, text and footer share one left edge, and the text is set larger (`MEMO_FOCUS_EDITOR_THEME`). Its header row shows only the memo's time on the left, as `TimestampPopover`: the time control once the memo has one (an edit, or a new memo dated from the calendar), otherwise "Now" in the viewer's language. The button that leaves the frame is on the right.

The session's code is a chunk of its own, loaded when a memo is first edited or composed, and renders without suspending once loaded. A host that knows an editor is coming calls `preloadMemoEditing()`: the router loads it with the Home page for a signed-in user, so the composer renders with the page, and the comment section loads it while the pointer is on its button. Until it is loaded a new draft shows a still of the empty composer, the same frame, placeholder line and toolbar height, so nothing moves when the session takes its place.

### Personal notes

New memos go into the personal workspace with private visibility. The service preserves the saved placement when editing a note from an older Space.

## Directory structure

```
MemoEditor/
├── index.ts                # MemoEditor, preloadMemoEditing, prop types
├── MemoEditor.tsx          # The memo-level component: modes, data, draft store, lazy session
├── MemoMarkdown.tsx        # The editor binding
├── useMemoDecorations.ts   # Tag colors and resolved mentions, one redecorate
├── view/                   # Read-only surface: MemoContentView, linkCards, headings, navigation, search highlight
└── edit/                   # Editing surface
    ├── EditSession.tsx         # The chrome over the draft store
    ├── Editor.tsx              # MemoMarkdown editable + composer extensions + controller
    ├── controller.ts, extensions.ts, uploadAnchors.ts, editor.css
    ├── draftKey.ts             # Identity → draft key, legacy migration
    ├── initialState.ts         # The store's first state, from the memo or the draft
    ├── state/, services/, hooks/, components/, Toolbar/, types/, utils/
    └── constants.ts
```

## Testing

- `tests/memo-record.test.tsx`: reads by name, freshness, optimistic patches, the comment mutations.
- `tests/browser/editor-style.spec.ts`: source-mode transparency and rich-text code backgrounds in Light, Dark and Paper; run `pnpm exec playwright test --config tests/browser/editor-style.config.ts` from `web/`.
- `tests/memo-editor-view.test.tsx`: view mode by name, task ticking through the mutation.
- `tests/memo-content-view.test.tsx`, `tests/editor.test.tsx`: the surfaces over the binding.
- `tests/use-memo-init.test.tsx`, `tests/draft-key.test.ts`, `tests/use-auto-save.test.tsx`: drafts and keys.
- `tests/use-memo-save.test.tsx`, `tests/memo-editor-*.test.ts*`: the session and its save path. Tests that mount the session call `await preloadMemoEditing()` first so it renders without suspending.
