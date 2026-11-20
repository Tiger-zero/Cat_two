# Themes

Color and font tokens come from `@usememos/ui` (read `web/node_modules/@usememos/ui/llms.txt`,
then its color.md). Every token is a CSS variable named `--mu-<name>`; the Tailwind utilities for them
carry an `mu-` prefix (`bg-mu-surface`, `text-mu-text-secondary`).

- **Light** pairs a warm gray canvas and white surfaces with mint selection and dark green actions.
  **Dark** uses charcoal surfaces and a lighter green accent. Both use the same component roles and are
  selected by `data-theme="light" | "dark"` on `<html>`.
  `utils/theme.ts` sets the attribute from the saved preference (`default`, `default-dark`, `paper` or
  `system`, kept as they are in the account setting and in localStorage) and the OS scheme.
- **Paper** (`paper.css`) is memos' app theme: a `[data-theme="paper"]` rule that must set every name in
  the package's `themeTokens` (29 colors), with every text pair at 4.5:1 or more.
  `tests/paper-theme.test.ts` checks both. Its primary stays ink, warm rather than a brand hue.
- `default.css` defines the Light and Dark overrides, the package's text fonts, and the wordmark's serif.
  The `memos-panel` utility in `index.css` uses the same surface and edge tokens across notes, the editor,
  statistics, and settings. The admin "Additional style" setting can target the same tokens and classes.

The warm surfaces and mint selection take visual inspiration from [Neno](https://github.com/openneno/neno).

There is no `.dark` or `.light` class; `color-scheme` comes from the theme rule, and the memo editor
follows it.
