# PicFetch — TODOs

## Done

- grid hover moves the keyboard position
  - Hovering a thumbnail moves GridWrap's own keyboard cursor with the ring,
    so the next arrow key steps on from the cell under the pointer.

- favorites menu
  - Save the currently open file list as a named collection.
  - Reopen and remove saved collections from a startup-populated menu.
  - Move removed collection folders to the OS recycle bin.

## TODO

 - favorites disk thumbnail cache
   Generate preview images in the background under each favorite's cache
   directory. When a favorite is loaded, let the grid read those previews and
   generate and persist only missing entries.

## not deemed worth implementing (edgecases)
