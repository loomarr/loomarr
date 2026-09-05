// Command prototype-prepared-admission is a throwaway TUI for the #1097 admission model.
package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	state := admissionState{Capacity: 4}
	input := bufio.NewScanner(os.Stdin)
	for {
		render(state)
		if !input.Scan() {
			return
		}
		switch input.Text() {
		case "b":
			state = state.addBackground(state.NextID + 1)
		case "u":
			state = state.addBackground(0)
		case "f":
			state = state.addForeground()
		case "r":
			state = state.releaseForeground()
		case "x":
			state = state.releaseBackground()
		case "q":
			return
		}
	}
}

func render(state admissionState) {
	fmt.Print("\033[2J\033[H")
	fmt.Println("\033[1mPROTOTYPE: prepared admission\033[0m")
	fmt.Printf("\033[1mcapacity\033[0m:   %d\n", state.Capacity)
	fmt.Printf("\033[1mforeground\033[0m: %d\n", state.Foreground)
	fmt.Printf("\033[1mbackground\033[0m: %v\n", state.Background)
	fmt.Printf("\033[1midle\033[0m:       %d\n", state.idle())
	fmt.Printf("\033[1mcancelled\033[0m:  %v\n", state.Cancelled)
	fmt.Println("\n\033[1m[b]\033[0m add later background  \033[1m[u]\033[0m add urgent background  \033[1m[f]\033[0m foreground arrives")
	fmt.Println("\033[1m[r]\033[0m foreground leaves       \033[1m[x]\033[0m background exits        \033[1m[q]\033[0m quit")
}
