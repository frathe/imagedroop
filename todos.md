# PicFetch — TODOs

## Done

- in grid view page down and page up keys can be used to navigate one page up or down.

## ACTIVE DEVELOPMENT

## TODO

 - Menu Items Export as PNG/JPEG should be combined to "Export image" menu item. Then the user is asked if he'd like to 
   export it as png or JPEG. The question should be keyboard enabled like the delete file confirmation. Also add a 
   keyboard shortcut CMD/CTRL + E
 - The set as Wallpaper menu Item should show the new to create keybinding CMD/CTRL + SHIFT + E
 - when zooming in or out the window scales in size with the image. The minimum windows size should not be smaller 
   then the default app open window size, and not larger then the user configurable maximum window size. 
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
