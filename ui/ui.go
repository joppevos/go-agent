// ui.go

package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Model for the menu (option selector)
type MenuModel struct {
	cursor   int
	choice   string
	quitting bool
}

func InitialMenuModel() MenuModel {
	return MenuModel{}
}

func (m MenuModel) Init() tea.Cmd {
	return nil
}

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < 1 {
				m.cursor++
			}
		case "enter":
			if m.cursor == 0 {
				m.choice = "Conversation Practice"
			} else {
				m.choice = "New Vocabulary"
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m MenuModel) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	options := []string{"Conversation Practice", "New Vocabulary"}
	s := "Choose an option:\n\n"
	for i, option := range options {
		cursor := " " // no cursor
		if m.cursor == i {
			cursor = ">" // cursor
		}
		s += fmt.Sprintf(" %s %s\n", cursor, option)
	}
	s += "\nPress ↑/↓ to move, enter to select."
	return s
}

// Accessor for choice
func (m MenuModel) Choice() string {
	return m.choice
}

// --- Spinner for loading/inference ---

type spinnerModel struct {
	spinner  spinner.Model
	quitting bool
}

func NewSpinnerModel() spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return spinnerModel{spinner: s}
}

func (m spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		return m, nil
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m spinnerModel) View() string {
	return fmt.Sprintf("%s Thinking...", m.spinner.View())
}

// Function to run the spinner
func RunSpinner() {
	p := tea.NewProgram(NewSpinnerModel())
	go func() {
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running spinner: %v", err)
			return
		}
	}()
}

// Function to manually sleep while showing spinner
func ShowSpinnerFor(duration time.Duration) {
	done := make(chan struct{})

	p := tea.NewProgram(NewSpinnerModel())
	go func() {
		p.Run()
		close(done)
	}()

	time.Sleep(duration)
	p.Quit()
	<-done
}
