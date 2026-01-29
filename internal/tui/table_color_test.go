package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/muesli/termenv"
)

func TestLipglossTableStyleFuncForeground(t *testing.T) {
	// Force TrueColor so test environment (no TTY) still produces ANSI codes.
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii) // restore

	red := lipgloss.Color("#ff0000")
	green := lipgloss.Color("#00ff00")

	rows := [][]string{
		{"hello", "world"},
		{"foo", "bar"},
	}

	tbl := table.New().
		Headers("COL1", "COL2").
		Rows(rows...).
		Width(40).
		Border(lipgloss.NormalBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(true).
		StyleFunc(func(row, col int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				return style.Bold(true)
			}
			if col == 0 {
				return style.Foreground(red)
			}
			return style.Foreground(green)
		})

	output := tbl.String()
	fmt.Printf("=== TABLE OUTPUT (raw) ===\n%s\n", output)
	fmt.Printf("=== TABLE OUTPUT (escaped) ===\n%q\n", output)

	if !strings.Contains(output, "\x1b[") {
		t.Error("table output contains NO ANSI escape codes at all — StyleFunc foreground is not being applied")
	}

	// Check for the red color (255;0;0 in true color)
	if !strings.Contains(output, "255;0;0") {
		t.Errorf("table output does not contain expected red color code 255;0;0")
	}
}

func TestLipglossTableStyleFuncWithBackground(t *testing.T) {
	// Force TrueColor so test environment (no TTY) still produces ANSI codes.
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	red := lipgloss.Color("#ff0000")
	bg := lipgloss.Color("#1a1b26")

	rows := [][]string{
		{"error-pod", "Error"},
		{"ok-pod", "Running"},
	}

	tbl := table.New().
		Headers("NAME", "STATUS").
		Rows(rows...).
		Width(40).
		Border(lipgloss.NormalBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(true).
		StyleFunc(func(row, col int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				return style
			}
			style = style.Background(bg)
			if col == 1 {
				style = style.Foreground(red)
			}
			return style
		})

	output := tbl.String()
	fmt.Printf("=== BG+FG TABLE (raw) ===\n%s\n", output)
	fmt.Printf("=== BG+FG TABLE (escaped) ===\n%q\n", output)

	if !strings.Contains(output, "\x1b[") {
		t.Error("table output contains NO ANSI escape codes — Background+Foreground combo is broken")
	}

	// Verify red foreground is present alongside background
	if !strings.Contains(output, "255;0;0") {
		t.Errorf("table output does not contain expected red foreground 255;0;0 when background is also set")
	}
}
