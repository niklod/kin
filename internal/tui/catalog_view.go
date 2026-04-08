package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/niklod/kin/internal/daemon"
)

// CatalogView renders the catalog panel.
func CatalogView(m CatalogModel, focused bool) string {
	var b strings.Builder

	visible := m.Visible()
	end := min(m.offset+m.height, len(visible))

	for i := m.offset; i < end; i++ {
		f := visible[i]
		line := formatFileEntry(f, m.width-4) // account for border padding

		if m.IsSelected(i) {
			line = checkboxOn + " " + line
		} else {
			line = checkboxOff + " " + line
		}

		if i == m.cursor {
			line = selectedItemStyle.Width(m.width - 4).Render(line)
		} else if !f.IsLocal {
			line = dimItemStyle.Render(line)
		} else {
			line = normalItemStyle.Render(line)
		}

		b.WriteString(line)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	// Pad remaining lines to fill height.
	rendered := strings.Count(b.String(), "\n") + 1
	if len(visible) == 0 {
		b.WriteString(dimItemStyle.Render("  (no files)"))
		rendered = 1
	}
	for i := rendered; i < m.height; i++ {
		b.WriteByte('\n')
	}

	content := b.String()

	// Search line at top if active.
	header := titleStyle.Render(" Files")
	if m.searching {
		header += "  " + dimItemStyle.Render("/") + m.search + dimItemStyle.Render("_")
	}
	if len(visible) > 0 {
		header += dimItemStyle.Render(fmt.Sprintf("  %d/%d", len(visible), len(m.files)))
	}

	border := borderStyle
	if focused {
		border = focusedBorderStyle
	}

	return border.Width(m.width).Render(
		header + "\n" + content,
	)
}

// formatFileEntry formats a single file entry line.
func formatFileEntry(f daemon.FileInfo, maxWidth int) string {
	size := formatSize(f.Size)
	loc := "remote"
	if f.IsLocal {
		loc = "local"
	}

	name := f.Name
	// Truncate name if too long.
	suffix := fmt.Sprintf("  %8s  %s", size, loc)
	available := maxWidth - lipgloss.Width(suffix) - 6 // checkbox + spaces
	if available > 0 && len(name) > available {
		name = name[:available-1] + "~"
	}

	return fmt.Sprintf("%-*s%s", max(0, available), name, suffix)
}

// formatSize returns a human-readable file size.
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
