package ui

import (
	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/filesort"
)

type appState struct {
	files         []fyne.URI
	unsortedFiles []fyne.URI
	index         int
	sortMode      filesort.Mode
	mergeMode     bool
}

func newAppState(sortMode filesort.Mode, mergeMode bool) appState {
	return appState{sortMode: sortMode, mergeMode: mergeMode}
}

func (s *appState) SortMode() filesort.Mode {
	return s.sortMode
}

func (s *appState) SetSortMode(mode filesort.Mode) {
	s.sortMode = mode
}

func (s *appState) MergeMode() bool {
	return s.mergeMode
}

func (s *appState) SetMergeMode(on bool) {
	s.mergeMode = on
}

func (s *appState) setFiles(unsorted, files []fyne.URI) {
	s.unsortedFiles = append([]fyne.URI(nil), unsorted...)
	s.files = append([]fyne.URI(nil), files...)
}

func (s *appState) replaceFiles(unsorted, files []fyne.URI) {
	s.setFiles(unsorted, files)
	s.index = 0
}

func (s *appState) clearFiles() {
	s.files = nil
	s.unsortedFiles = nil
	s.index = 0
}

func (s *appState) removeFile(i int) fyne.URI {
	target := s.files[i]
	s.files = append(s.files[:i], s.files[i+1:]...)
	if s.index >= len(s.files) {
		s.index = len(s.files) - 1
	}

	for j, u := range s.unsortedFiles {
		if u.String() == target.String() {
			s.unsortedFiles = append(s.unsortedFiles[:j], s.unsortedFiles[j+1:]...)
			break
		}
	}

	return target
}
