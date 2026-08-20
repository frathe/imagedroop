# PicFetch — TODOs

## Done

 - favorites disk thumbnail cache
   Favorite grid previews are now generated in the background on save and
   topped up on open, cached under each favorite's own `thumbs/` folder keyed
   by source mtime+size, and swept for stale/removed entries afterward
   (`internal/favthumbs`, wired through `internal/ui/favthumbs.go`).

## ACTIVE DEVELOPMENT

## TODO

 - There is a bug in the windows Version: WHen in Gridview, multiselect via
   space key works, but when trying it with mouse and Ctrl key it does not.
   Holding the Ctrl key down and clicking on a image does not select it but 
   instead opens it. Observation when pushing ctrl key at exatly the same time
   as clicking on the image it actually works and the image is selected.

## not deemed worth implementing (edgecases)
