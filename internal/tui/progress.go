package tui

import (
	"fmt"
	"strings"

	"github.com/niklod/kin/internal/daemon"
)

// ProgressModel tracks active file transfers.
type ProgressModel struct {
	transfers []transferState
	visible   bool
	width     int
}

type transferState struct {
	fileID   string
	fileName string
	done     bool
	err      error
}

// SetWidth updates the available width.
func (m *ProgressModel) SetWidth(w int) {
	m.width = w
}

// Toggle flips the visibility of the progress view.
func (m *ProgressModel) Toggle() {
	m.visible = !m.visible
}

// Visible returns whether the progress view is shown.
func (m ProgressModel) Visible() bool {
	return m.visible
}

// AddTransfer starts tracking a new download.
func (m *ProgressModel) AddTransfer(f daemon.FileInfo) {
	m.transfers = append(m.transfers, transferState{
		fileID:   f.FileID,
		fileName: f.Name,
	})
}

// MarkDone marks a transfer as completed.
func (m *ProgressModel) MarkDone(fileID string, err error) {
	for i, t := range m.transfers {
		if t.fileID == fileID {
			m.transfers[i].done = true
			m.transfers[i].err = err
			return
		}
	}
}

// HasActive returns true if any transfers are in progress.
func (m ProgressModel) HasActive() bool {
	for _, t := range m.transfers {
		if !t.done {
			return true
		}
	}
	return false
}

// View renders the progress panel.
func (m ProgressModel) View() string {
	if !m.visible || len(m.transfers) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(" Transfers") + "\n")

	for _, t := range m.transfers {
		status := dimItemStyle.Render("downloading...")
		if t.done {
			if t.err != nil {
				status = errorStyle.Render(fmt.Sprintf("error: %v", t.err))
			} else {
				status = statusKeyStyle.Render("done")
			}
		}
		b.WriteString(fmt.Sprintf("  %s  %s\n", t.fileName, status))
	}

	return borderStyle.Width(m.width).Render(b.String())
}
