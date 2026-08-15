# Image Drop — TODOs

## Done

## TODO

- **Cancel pending decodes when new files are dropped** — attach a
   `context.Context`/`cancel` to each decode so an abandoned load stops
   doing I/O instead of finishing unseen; the `gen` guard only hides the
   result, it doesn't stop the work.

## not deemed worth implementing (edgecases)
