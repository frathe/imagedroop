# PicFetch — TODOs

## Done

- favorites menu
  - Save the currently open file list as a named collection.
  - Reopen and remove saved collections from a startup-populated menu.
  - Move removed collection folders to the OS recycle bin.

## TODO

- hideConsoleWindow is duplicated 4 times across clipboard,
 filepicker, trash, wallpaper. Let's refactor this.

- The viewer struct is a "god struct" (~803 lines)
 Group related fields into small composition types. For example:
 ```golang
    type menuState struct {
        saveItem       *fyne.MenuItem
        exportPNGItem  *fyne.MenuItem
        exportJPEGItem *fyne.MenuItem
        wallpaperItem  *fyne.MenuItem
        closeFilesItem *fyne.MenuItem
    }

    type navigationState struct {
        files         []fyne.URI
        index         int
        unsortedFiles []fyne.URI
        sortMode      filesort.Mode
        mergeMode     bool
        baseTitle     string
        gen           atomic.Uint64
        loadCancel    context.CancelFunc
    }
 ```

- preferences.go is becoming a config god-type
 Split State into domain-specific sub-types that compose:
 ```golang
    type State struct {
        Sort        SortState
        Merge       bool
        Slideshow   SlideshowState
        Scan        ScanState
        Window      WindowState
        Cache       CacheState
        Geometry    GeometryState
    }

    type SlideshowState struct {
        Interval   time.Duration
        Shuffle    bool
    }

    type CacheState struct {
        MaxImageCacheMB int
        MaxThumbCacheMB int
        MaxFileSizeMB   int
    }
 ```

 - favorites disk thumbnail cache
   Generate preview images in the background under each favorite's cache
   directory. When a favorite is loaded, let the grid read those previews and
   generate and persist only missing entries.

## not deemed worth implementing (edgecases)
