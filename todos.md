# PicFetch — TODOs

## Done

 - favorites’ disk thumbnail cache
   Favorite grid previews are now generated in the background on save and
   topped up on open, cached under each favorite's own `thumbs/` folder keyed
   by source mtime+size, and swept for stale/removed entries afterward
   (`internal/favthumbs`, wired through `internal/ui/favthumbs.go`).

## ACTIVE DEVELOPMENT

## TODO

## not deemed worth implementing (edge cases)

- There is a bug in the Windows Version: WHen in Gridview, multiselect via
  the space key works, but when trying it with mouse and Ctrl key, it does not.
  Holding the Ctrl key down and clicking on an image does not select it but
  instead opens it. Observation, when pushing the Ctrl key at exactly the same time
  as clicking on the image, it actually works, and the image is selected.
  (this seems to be a bug in fyne, created an issue, sorry Windows users)