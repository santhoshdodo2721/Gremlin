package tui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Cyan   = "\033[36m"
	Gray   = "\033[90m"
)

var reader = bufio.NewReader(os.Stdin)

func Clear() {
	fmt.Print("\033[H\033[2J")
}

func Banner() {
	title := "  G R E M L I N - I N - A - B O X  "
	fmt.Println()
	fmt.Print(Red + Bold)
	for _, ch := range title {
		fmt.Printf("%c", ch)
		time.Sleep(8 * time.Millisecond)
	}
	fmt.Println(Reset)
	fmt.Println(Gray + "  chaos engineering, interactively" + Reset)
	fmt.Println()
}

func Menu(title string, options []string) int {
	fmt.Println(Bold + Cyan + title + Reset)
	fmt.Println(strings.Repeat("-", len(title)))
	for i, opt := range options {
		fmt.Printf("  %s%d%s) %s\n", Yellow, i+1, Reset, opt)
	}
	fmt.Printf("  %sq%s) back / quit\n\n", Yellow, Reset)
	fmt.Print("select> ")

	line, _ := reader.ReadString(10)
	line = strings.TrimSpace(line)
	if line == "q" || line == "quit" {
		return -1
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(options) {
		fmt.Println(Red + "invalid selection" + Reset)
		return -1
	}
	return n - 1
}

func Ask(label string) string {
	fmt.Printf("%s: ", label)
	line, _ := reader.ReadString(10)
	return strings.TrimSpace(line)
}

func AskDefault(label, def string) string {
	fmt.Printf("%s [%s]: ", label, def)
	line, _ := reader.ReadString(10)
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func Spin(label string, work func() error) error {
	frames := []string{"|", "/", "-", "+"}
	done := make(chan error, 1)
	start := time.Now()

	go func() {
		done <- work()
	}()

	i := 0
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			elapsed := time.Since(start).Round(time.Millisecond)
			fmt.Print("\r")
			if err != nil {
				fmt.Printf("%s[x]%s %s failed after %s: %v          \n", Red, Reset, label, elapsed, err)
			} else {
				fmt.Printf("%s[ok]%s %s finished in %s          \n", Green, Reset, label, elapsed)
			}
			return err
		case <-ticker.C:
			fmt.Printf("\r%s%s%s %s...          ", Yellow, frames[i%len(frames)], Reset, label)
			i++
		}
	}
}

func Pause() {
	fmt.Print("\npress enter to continue...")
	reader.ReadString(10)
}
