package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/jmelahman/connections/game"
)

func main() {
	screen, err := tcell.NewConsoleScreen()
	if err != nil {
		screen, err = tcell.NewScreen()
	}
	if err != nil {
		panic(err)
	}

	if err := game.RunWithScreen(screen); err != nil {
		panic(err)
	}
}
