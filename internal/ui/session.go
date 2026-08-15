package ui

// restoreSession loads the file set saved when the window last closed. It's
// offered via restoreLink rather than restored automatically - silently
// reloading a possibly large, or by now partly missing, set on every launch
// would be surprising. A missing file is handled the same as any other
// broken file, via attemptLoad's existing retry chain, so no separate
// existence check is needed here.
func (v *viewer) restoreSession() {
	files := v.savedSession
	v.savedSession = nil
	v.restoreLink.Hide()

	v.handleDrop(files)
}
