# PicFetch — TODOs

## Done

- in grid view page down and page up keys can be used to navigate one page up or down.
- Menu Items Export as PNG/JPEG combined to a single "Export image" menu item; choosing it asks whether to export as
  PNG or JPEG through a keyboard-enabled prompt, the same shape as the delete file confirmation, also reachable via
  the new keyboard shortcut CMD/CTRL + E.
- The Set as Wallpaper menu item shows its new keybinding CMD/CTRL + SHIFT + E.
- when zooming in or out the window scales in size with the image. The minimum window size is the default app
  open size, and the maximum is the user-configurable maximum window size; pan inside the window once the cap
  is reached.
- the manual has a search bar at the top of the window ("Search for..."). Enter highlights matches and scrolls
  the first into view; Enter again on the same term walks to the next match. Help → Manual shows the F1 key.
- the manual's search box has a hidden trigger: submitting one exact phrase searches for nothing and instead
  opens a full-screen animated shader window with two patterns (N), status/help/FPS overlays, mouse-follow (F)
  and a slider panel. Escape closes just that window, leaving the app running. Ported from
  ../golang_course/spiral/ into internal/ui/spiral; the phrase itself is `secretPhrase` in
  internal/ui/help/manual.go, left unwritten here so it stays worth finding.

## ACTIVE DEVELOPMENT

## TODO

 - favorites disk thumbnail cache
   Generate preview images in the background under each favorite's cache
   directory. When a favorite is loaded, let the grid read those previews and
   generate and persist only missing entries.

## not deemed worth implementing (edgecases)
