# Themes

Color and font tokens come from `@usememos/ui` (read `web/node_modules/@usememos/ui/llms.txt`,
then its color.md). Every token is a CSS variable named `--mu-<name>`; the Tailwind utilities for them
carry an `mu-` prefix (`bg-mu-surface`, `text-mu-text-secondary`).

- **Light** pairs a gray-green canvas and warm white shared surfaces with dark green actions.
  The desktop home sidebar, account panel, calendar, composer, and note cards use `#FFFFFF`.
  **Dark** uses `#121212` for the canvas, sidebar and shared surfaces, with lighter green actions.
  Both use the same component roles and are selected by `data-theme="light" | "dark"` on `<html>`.
  `utils/theme.ts` sets the attribute from the saved preference (`default`, `default-dark`, `paper` or
  `system`, kept as they are in the account setting and in localStorage) and the OS scheme.
- **Paper** (`paper.css`) is memos' app theme: a `[data-theme="paper"]` rule that must set every name in
  the package's `themeTokens` (29 colors), with every text pair at 4.5:1 or more.
  `tests/paper-theme.test.ts` checks both. Its primary stays ink, warm rather than a brand hue.
- `default.css` defines the Light and Dark overrides, the package's text fonts, and the wordmark's serif.
  Sidebar, selected rows, composer focus, inline memo tags and calendar levels have dedicated roles, with
  semantic fallbacks for Paper and forced colors. The `memos-panel` utility in `index.css` uses the same
  surface and edge tokens across notes, the editor and settings.

See the [desktop design and palette](../../../.agents/skills/cat-two-ui-development/references/desktop-visual-design.md)
for design references, swatches and contrast checks.

There is no `.dark` or `.light` class; `color-scheme` comes from the theme rule, and the memo editor
follows it.
