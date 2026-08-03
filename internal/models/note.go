package models

import "time"

// Note represents a note with its metadata
type Note struct {
	Filename string    `json:"filename"`
	Title    string    `json:"title"`
	Content  string    `json:"content"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
}

// CreateNoteRequest represents a request to create or update a note
type CreateNoteRequest struct {
	Title            string `json:"title"`
	Content          string `json:"content"`
	OriginalFilename string `json:"original_filename,omitempty"`
}

// NoteList represents a list of notes
type NoteList struct {
	Notes []Note `json:"notes"`
}
