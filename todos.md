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

## ACTIVE DEVELOPMENT

## TODO

 - the manual should have a search bar. searches text should be highlighted in the text and when hitting enter the 
   search should be performed, the window should scroll the first result into view. when hitting enter again on the same
   search term the second result should scroll into view and so on. The search bar has a fixed position on the top of
   the window. and has a helpful text like "Search for...", The Help key F1 should also be shown in the help menu.
 - We need an easter egg! When searching the manual for "please hypnotize me" a full screen window should be opened 
   The project code for the spiral easter egg is in the folder ../golang_course/spiral/ adapt it so it can live in
   this repository.
 - favorites disk thumbnail cache
   Generate preview images in the background under each favorite's cache
   directory. When a favorite is loaded, let the grid read those previews and
   generate and persist only missing entries.

## not deemed worth implementing (edgecases)
