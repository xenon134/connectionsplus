package game

import (
	"fmt"
	"maps"
	"math/rand"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	currentDate    time.Time
	puzzleResponse Response
	nextDate       time.Time // zero = exit, non-zero = load that date
)

type GameState struct {
	selectedCards   map[string]bool
	categories      map[string]Group
	currentMatchRow int
	history         []string // One emoji row per submitted guess.
	mistakes        int
	wrongGuesses    map[string]bool // Distinct incorrect guesses, keyed by emoji row.
}

var tileEmoji = [4]string{"🟨", "🟩", "🟦", "🟪"}

// copyToClipboard copies text using the platform's clipboard utility.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func RunWithScreen(screen tcell.Screen) error {
	loadProgress()
	if err := loadPuzzleForDate(time.Now()); err != nil {
		return err
	}

	app := tview.NewApplication()
	app.SetScreen(screen)

	var loading bool
	var navigate func(d time.Time)
	navigate = func(d time.Time) {
		if loading {
			return
		}
		loading = true
		go func() {
			err := loadPuzzleForDate(d)
			if err != nil {
				app.QueueUpdateDraw(func() { loading = false })
				return
			}
			app.QueueUpdateDraw(func() {
				root := buildUI(app, screen, navigate)
				app.SetRoot(root, true)
				loading = false
			})
		}()
	}

	root := buildUI(app, screen, navigate)
	return app.SetRoot(root, true).EnableMouse(true).Run()
}

func buildUI(app *tview.Application, screen tcell.Screen, navigate func(time.Time)) tview.Primitive {
	gameState := GameState{
		selectedCards: make(map[string]bool),
		categories:    make(map[string]Group),
		wrongGuesses:  make(map[string]bool),
	}

	response := puzzleResponse
	date := currentDate

	// The Connections puzzle number is derived from the print date: puzzle #1
	// was 2023-06-12. Used by the header and the share string.
	puzzleNumber := int(date.Sub(time.Date(2023, 6, 12, 0, 0, 0, 0, time.UTC)).Hours()/24) + 1

	grid := tview.NewGrid().
		SetRows(3, 3, 3, 3, 3). // Extra row for submit button.
		SetColumns(20, 20, 20, 20)

	buttons := [4][4]*tview.Button{}
	var focusedRow, focusedCol int

	var selectedStyle tcell.Style
	if screen.Colors() < 256 {
		selectedStyle = selectedStyle.Bold(true).Underline(true)
	} else {
		selectedStyle = selectedStyle.Foreground(tcell.ColorGray)
	}
	disabledStyle := tcell.StyleDefault.Foreground(tcell.ColorDarkGray).StrikeThrough(true)

	var shuffleButton, submitButton, deselectButton, shareButton *tview.Button
	var contentFlex *tview.Flex

	mistakesText := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("Mistakes Remaining: ● ● ● ●")

	gameOver := false
	replaying := false

	prevBtn := tview.NewButton("<").
		SetSelectedFunc(func() {
			navigate(currentDate.AddDate(0, 0, -1))
		}).
		SetStyle(tcell.StyleDefault).
		SetActivatedStyle(selectedStyle)

	nextBtn := tview.NewButton(">").
		SetSelectedFunc(func() {
			navigate(currentDate.AddDate(0, 0, 1))
		}).
		SetStyle(tcell.StyleDefault).
		SetActivatedStyle(selectedStyle)

	updateMistakes := func() {
		if remaining := 4 - gameState.mistakes; remaining > 0 {
			mistakesText.SetText(fmt.Sprintf("Mistakes Remaining: %s", strings.TrimRight(strings.Repeat("● ", remaining), " ")))
		} else {
			mistakesText.SetText(fmt.Sprintf("Mistakes: %d", gameState.mistakes))
		}
	}

	resetSubmitButton := func() {
		if len(gameState.selectedCards) != 4 {
			submitButton.SetStyle(disabledStyle).SetActivatedStyle(disabledStyle)
		} else {
			submitButton.SetStyle(tcell.StyleDefault).SetActivatedStyle(selectedStyle)
		}
		submitButton.SetLabel("Submit (s)")
	}

	findButton := func(r, c int) *tview.Button {
		if r == -1 {
			if c == 0 {
				return prevBtn
			}
			return nextBtn
		}
		if r == 4 {
			if gameOver {
				return shareButton
			}
			switch c {
			case 0:
				return shuffleButton
			case 1, 2:
				return submitButton
			case 3:
				return deselectButton
			}
		}
		return buttons[r][c]
	}

	setFocus := func(r, c int) {
		// Unset previous button's border.
		if focusedRow < 4 && focusedRow >= 0 {
			findButton(focusedRow, focusedCol).SetBorderColor(tcell.ColorDarkGray)
		}
		focusedRow = r
		focusedCol = c
		// Set current button's border.
		button := findButton(focusedRow, focusedCol)
		if focusedRow < 4 && focusedRow >= 0 {
			button.SetBorderColor(tcell.ColorGray)
		}
		app.SetFocus(button)
	}

	handleClick := func(r, c int) func() {
		return func() {
			label := buttons[r][c].GetLabel()
			if gameState.selectedCards[label] {
				delete(gameState.selectedCards, label)
				buttons[r][c].SetStyle(tcell.StyleDefault).SetActivatedStyle(tcell.StyleDefault)
			} else if len(gameState.selectedCards) < 4 {
				gameState.selectedCards[label] = true
				buttons[r][c].SetStyle(selectedStyle).SetActivatedStyle(selectedStyle)
			} else {
				return
			}
			setFocus(r, c)
			resetSubmitButton()
		}
	}

	handleDeselect := func() {
		deselectButton.SetActivatedStyle(selectedStyle)
		for cardContent := range gameState.selectedCards {
			delete(gameState.selectedCards, cardContent)
		}
		for i := range 4 {
			for j := range 4 {
				buttons[i][j].SetStyle(tcell.StyleDefault).SetActivatedStyle(tcell.StyleDefault)
			}
		}
		resetSubmitButton()
	}

	handleShuffle := func() {
		shuffleButton.SetActivatedStyle(selectedStyle)
		// Capture the focused button before the shuffle moves it to a new cell.
		var focusedButton *tview.Button
		if focusedRow < 4 {
			focusedButton = findButton(focusedRow, focusedCol)
		}
		// Flatten the buttons array for rows greater than currentMatchRow into a slice for shuffling
		var flatButtons []*tview.Button
		for i := gameState.currentMatchRow; i < 4; i++ {
			for j := range 4 {
				flatButtons = append(flatButtons, buttons[i][j])
			}
		}

		// Shuffle the flatButtons slice
		rand.Shuffle(len(flatButtons), func(i, j int) {
			flatButtons[i], flatButtons[j] = flatButtons[j], flatButtons[i]
		})

		// Reassign the shuffled buttons back to the grid
		index := 0
		for i := gameState.currentMatchRow; i < 4; i++ {
			for j := range 4 {
				button := flatButtons[index].SetSelectedFunc(handleClick(i, j))
				index++
				grid.RemoveItem(button)
				grid.AddItem(button, i, j, 1, 1, 0, 0, false)
				buttons[i][j] = button
			}
		}

		// The focused word's button moved to a new cell with its focus border,
		// so point the focus bookkeeping at its new position.
		if focusedButton != nil {
			for i := gameState.currentMatchRow; i < 4; i++ {
				for j := range 4 {
					if buttons[i][j] == focusedButton {
						focusedRow, focusedCol = i, j
					}
				}
			}
		}
		resetSubmitButton()
	}

	handleShare := func() {
		var result strings.Builder
		result.WriteString("Connections\n")
		result.WriteString(fmt.Sprintf("Puzzle #%d\n", puzzleNumber))
		for _, row := range gameState.history {
			result.WriteString(row)
			result.WriteByte('\n')
		}
		if err := copyToClipboard(result.String()); err != nil {
			shareButton.SetLabel(fmt.Sprintf("Copy failed: %v", err))
			return
		}
		shareButton.SetLabel("Copied to clipboard!")
	}

	handleSubmit := func() {
		if len(gameState.selectedCards) != 4 {
			return
		}

		words := slices.Collect(maps.Keys(gameState.selectedCards))

		slices.SortStableFunc(words, func(a, b string) int {
			if ai, bi := gameState.categories[a].Index, gameState.categories[b].Index; ai != bi {
				return ai - bi
			}
			return strings.Compare(a, b)
		})

		guessKey := strings.Join(words, ",")
		if gameState.wrongGuesses[guessKey] {
			submitButton.
				SetStyle(tcell.StyleDefault.Background(tcell.ColorRed).Foreground(tcell.ColorBlack.TrueColor())).
				SetActivatedStyle(tcell.StyleDefault.Background(tcell.ColorRed).Foreground(tcell.ColorBlack.TrueColor())).
				SetLabel("Already Guessed")
			return
		}

		if !replaying {
			dateKey := currentDate.Format("2006-01-02")
			progress[dateKey] = append(progress[dateKey], slices.Clone(words))
			saveProgress()
		}

		row := ""
		for _, w := range words {
			row += tileEmoji[gameState.categories[w].Index]
		}
		gameState.history = append(gameState.history, row)

		var categoryTitle string
		var categoryIndex int
		categoryMap := make(map[string](int))
		const (
			correct = iota
			offByOne
			incorrect
		)
		result := incorrect

		for cardContent := range gameState.selectedCards {
			categoryIndex = gameState.categories[cardContent].Index
			categoryTitle = gameState.categories[cardContent].Title
			categoryMap[categoryTitle]++
			switch categoryMap[categoryTitle] {
			case 3:
				result = offByOne
			case 4:
				result = correct
			}
		}

		switch result {
		case correct:
			contents := fmt.Sprintf(
				"%s: %s",
				categoryTitle,
				strings.Join(slices.Collect(maps.Keys(gameState.selectedCards)), ", "),
			)
			button := tview.NewButton(contents).SetDisabled(true)
			switch categoryIndex {
			case 0:
				button.SetDisabledStyle(tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlack.TrueColor()))
			case 1:
				button.SetDisabledStyle(tcell.StyleDefault.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack.TrueColor()))
			case 2:
				button.SetDisabledStyle(tcell.StyleDefault.Background(tcell.ColorBlue).Foreground(tcell.ColorBlack.TrueColor()))
			case 3:
				button.SetDisabledStyle(tcell.StyleDefault.Background(tcell.ColorPurple).Foreground(tcell.ColorBlack.TrueColor()))
			}
			grid.AddItem(button, gameState.currentMatchRow, 0, 1, 4, 0, 0, false)

			// Collect all remaining buttons that were not selected
			var remainingButtons []*tview.Button
			for i := gameState.currentMatchRow; i < 4; i++ {
				for j := range 4 {
					button := buttons[i][j]
					if button == nil {
						continue
					}
					grid.RemoveItem(button)
					if !gameState.selectedCards[button.GetLabel()] {
						remainingButtons = append(remainingButtons, button)
					}
				}
			}
			// Place remaining buttons into the grid starting from currentMatchRow + 1
			remIdx := 0
			for i := gameState.currentMatchRow + 1; i < 4; i++ {
				for j := range 4 {
					if remIdx < len(remainingButtons) {
						btn := remainingButtons[remIdx].SetSelectedFunc(handleClick(i, j))
						buttons[i][j] = btn
						grid.AddItem(btn, i, j, 1, 1, 0, 0, false)
						remIdx++
					} else {
						buttons[i][j] = nil
					}
				}
			}
			if focusedRow == gameState.currentMatchRow {
				focusedRow++
			}
			gameState.currentMatchRow++
			for cardContent := range gameState.selectedCards {
				delete(gameState.selectedCards, cardContent)
			}

			if gameState.currentMatchRow == 4 {
				// Game over: swap the controls for a single share button.
				gameOver = true
				grid.RemoveItem(shuffleButton)
				grid.RemoveItem(submitButton)
				grid.RemoveItem(deselectButton)
				shareButton = tview.NewButton("Share Your Result").
					SetSelectedFunc(handleShare).
					SetStyle(tcell.StyleDefault.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack.TrueColor())).
					SetActivatedStyle(tcell.StyleDefault.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack.TrueColor()))
				grid.AddItem(shareButton, 4, 0, 1, 4, 0, 0, false)
				focusedRow, focusedCol = 4, 0
				app.SetFocus(shareButton)
			}

			submitButton.
				SetStyle(tcell.StyleDefault.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack.TrueColor())).
				SetActivatedStyle(tcell.StyleDefault.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack.TrueColor()))
		case offByOne:
			gameState.wrongGuesses[guessKey] = true
			gameState.mistakes++
			updateMistakes()
			submitButton.
				SetStyle(tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlack.TrueColor())).
				SetActivatedStyle(tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlack.TrueColor())).
				SetLabel("One away...")
		default:
			gameState.wrongGuesses[guessKey] = true
			gameState.mistakes++
			updateMistakes()
			submitButton.
				SetStyle(tcell.StyleDefault.Background(tcell.ColorRed).Foreground(tcell.ColorBlack.TrueColor())).
				SetActivatedStyle(tcell.StyleDefault.Background(tcell.ColorRed).Foreground(tcell.ColorBlack.TrueColor())).
				SetLabel("Incorrect")
		}
	}

	for row := range 4 {
		category := response.Categories[row]
		for col := range 4 {
			card := category.Cards[col]
			label := cases.Title(language.AmericanEnglish).String(card.Content)
			title := cases.Title(language.AmericanEnglish).String(category.Title)
			gRow := card.Position / 4
			gCol := card.Position % 4

			gameState.categories[label] = Group{title, row}

			button := tview.NewButton(label).
				SetSelectedFunc(handleClick(gRow, gCol)).
				SetStyle(tcell.StyleDefault).
				SetActivatedStyle(tcell.StyleDefault)
			button.SetBorder(true).SetBorderColor(tcell.ColorDarkGray)
			buttons[gRow][gCol] = button
			grid.AddItem(button, gRow, gCol, 1, 1, 0, 0, false)
		}
	}

	shuffleButton = tview.NewButton("Shuffle (a)").
		SetSelectedFunc(handleShuffle).
		SetStyle(tcell.StyleDefault).
		SetActivatedStyle(selectedStyle)

	submitButton = tview.NewButton("Submit (s)").
		SetSelectedFunc(handleSubmit).
		SetStyle(disabledStyle).
		SetActivatedStyle(disabledStyle)

	deselectButton = tview.NewButton("Deselect All (d)").
		SetSelectedFunc(handleDeselect).
		SetStyle(tcell.StyleDefault).
		SetActivatedStyle(selectedStyle)

	grid.AddItem(shuffleButton, 4, 0, 1, 1, 0, 0, false)
	grid.AddItem(submitButton, 4, 1, 1, 2, 0, 0, false)
	grid.AddItem(deselectButton, 4, 3, 1, 1, 0, 0, false)

	// Replay saved submissions history for the current date
	dateKey := currentDate.Format("2006-01-02")
	if pastSubmissions, ok := progress[dateKey]; ok {
		replaying = true
		for _, guess := range pastSubmissions {
			if len(guess) != 4 || gameOver {
				continue
			}
			for _, w := range guess {
				gameState.selectedCards[w] = true
			}
			handleSubmit()
			clear(gameState.selectedCards)
		}
		replaying = false
		// Reset submit button state after replay
		resetSubmitButton()
	}

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		r := focusedRow
		c := focusedCol

		switch {
		case event.Key() == tcell.KeyRune && event.Rune() == 'q':
			app.Stop()
		case event.Key() == tcell.KeyRune && event.Rune() == 'a' && !gameOver:
			handleShuffle()
			r, c = focusedRow, focusedCol
		case event.Key() == tcell.KeyRune && event.Rune() == 's' && !gameOver:
			handleSubmit()
			if r < gameState.currentMatchRow {
				r++
			}
		case event.Key() == tcell.KeyRune && event.Rune() == 'd' && !gameOver:
			handleDeselect()
		case event.Key() == tcell.KeyUp, event.Key() == tcell.KeyRune && event.Rune() == 'k':
			if r > gameState.currentMatchRow {
				r--
			} else if r == gameState.currentMatchRow {
				r = -1
				c = 0
			}
			resetSubmitButton()
		case event.Key() == tcell.KeyDown, event.Key() == tcell.KeyRune && event.Rune() == 'j':
			if r == -1 {
				r = gameState.currentMatchRow
				c = 0
			} else if r < 4 {
				r++
			}
			resetSubmitButton()
		case event.Key() == tcell.KeyLeft, event.Key() == tcell.KeyRune && event.Rune() == 'h':
			if r == -1 {
				c = 0
			} else if c > 0 {
				if r == 4 && c == 2 {
					c--
				}
				c--
			}
			resetSubmitButton()
		case event.Key() == tcell.KeyRight, event.Key() == tcell.KeyRune && event.Rune() == 'l':
			if r == -1 {
				c = 1
			} else if c < 3 {
				if r == 4 && c == 1 {
					c++
				}
				c++
			}
			resetSubmitButton()
		case event.Key() == tcell.KeyEnter, event.Key() == tcell.KeyRune && event.Rune() == ' ':
			if r == -1 {
				if c == 0 {
					navigate(currentDate.AddDate(0, 0, -1))
				} else {
					navigate(currentDate.AddDate(0, 0, 1))
				}
				return nil
			} else if r == 4 {
				if gameOver {
					handleShare()
					break
				}
				switch c {
				case 0:
					handleShuffle()
				case 3:
					handleDeselect()
				default:
					handleSubmit()
				}
			} else {
				label := buttons[r][c].GetLabel()
				if gameState.selectedCards[label] {
					delete(gameState.selectedCards, label)
					buttons[r][c].SetStyle(tcell.StyleDefault).SetActivatedStyle(tcell.StyleDefault)
				} else if len(gameState.selectedCards) < 4 {
					gameState.selectedCards[label] = true
					buttons[r][c].SetStyle(selectedStyle).SetActivatedStyle(selectedStyle)
				}
				resetSubmitButton()
			}
		default:
			return event
		}

		setFocus(r, c)
		return nil
	})

	setFocus(0, 0)

	headerText := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText(fmt.Sprintf("Connections #%d\n%s %d, %d", puzzleNumber, date.Format("January"), date.Day(), date.Year()))

	headerRow := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(prevBtn, 3, 0, false).
		AddItem(headerText, 0, 1, false).
		AddItem(nextBtn, 3, 0, false)

	// The game content needs 19 rows: header (2), gap (1), grid (15), and the
	// mistakes counter (1). Extra terminal rows are split evenly above and
	// below, with an odd row going to the top. All sizes are fixed (recomputed
	// before each draw) so nothing drifts as the terminal is resized; only the
	// grid is proportional, so it compresses gracefully below 19 rows.
	contentFlex = tview.NewFlex().SetDirection(tview.FlexRow)
	topSpacer, bottomSpacer := tview.NewBox(), tview.NewBox()
	headerGap, footerGap := tview.NewBox(), tview.NewBox()
	// Rebuild the content column for the given terminal height. Called before
	// every draw (so resizing recenters the layout) and once up front: SetRoot
	// cascades focus down through the column, which only reaches the grid if
	// the items already exist.
	relayout := func(height int) {
		extra := max(height-19, 0)
		footer := tview.Primitive(mistakesText)
		if gameOver {
			footer = footerGap // Keep the row count stable once the counter disappears.
		}
		contentFlex.Clear().
			AddItem(topSpacer, extra/2+extra%2, 0, false).
			AddItem(headerRow, 2, 0, false).
			AddItem(headerGap, 1, 0, false).
			AddItem(grid, 0, 1, true).
			AddItem(footer, 1, 0, false).
			AddItem(bottomSpacer, extra/2, 0, false)
	}
	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		_, height := screen.Size()
		relayout(height)
		return false
	})
	_, termHeight := screen.Size()
	relayout(termHeight)
	flex := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false). // Left spacer.
		AddItem(contentFlex, 80, 1, true).    // The centered game column.
		AddItem(tview.NewBox(), 0, 1, false)  // Right spacer.

	return flex
}
