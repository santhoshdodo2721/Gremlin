package tui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Cyan   = "\033[36m"
	Gray   = "\033[90m"
)

var reader = bufio.NewReader(os.Stdin)

// Plain output works in pipes and honors the conventional NO_COLOR setting.
var interactive = terminalOutput()

func terminalOutput() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func init() {
	_, noColor := os.LookupEnv("NO_COLOR")
	if noColor || !interactive || os.Getenv("TERM") == "dumb" {
		Reset, Bold, Red, Green, Yellow, Cyan, Gray = "", "", "", "", "", "", ""
	}
}

func Clear() {
	if interactive && Reset != "" {
		fmt.Print("\033[H\033[2J")
	}
}

func Banner() {
	const logo = ` ____           _ _                     ___
|  _ \ ___  ___(_) | ___ _ __   ___ ___  / _ \ _ __  ___
| |_) / _ \/ __| | |/ _ \ '_ \ / __/ _ \| | | | '_ \/ __|
|  _ <  __/\__ \ | |  __/ | | | (_|  __/| |_| | |_) \__ \
|_| \_\___||___/_|_|\___|_| |_|\___\___| \___/| .__/|___/
                                             |_|`
	fmt.Println()
	// The requested brand color stays red in an interactive terminal,
	// including terminals that inherit NO_COLOR or TERM=dumb.
	bannerColor, bannerReset := "", ""
	if interactive {
		bannerColor, bannerReset = "\033[1;31m", "\033[0m"
	}
	fmt.Print(bannerColor)
	for _, line := range strings.Split(logo, "\n") {
		fmt.Println("  " + line)
	}
	fmt.Println(bannerReset)
	fmt.Println("  " + bannerColor + "ResilenceOps" + bannerReset + Gray + "  /  APPLICATION RESILIENCE CONSOLE  /  v0.1.0" + Reset)
	fmt.Println()
}

func MainMenu(options []string) int {
	return menu("CONTROL CENTER", options, "Exit")
}

func Menu(title string, options []string) int {
	return menu(title, options, "Back")
}

func menu(title string, options []string, exitLabel string) int {
	const width = 66
	fmt.Println()
	fmt.Println(Gray + "  +" + strings.Repeat("-", width) + "+" + Reset)
	fmt.Printf("%s  |%s %-64s %s|%s\n", Gray, Bold, strings.ToUpper(title), Gray, Reset)
	fmt.Println(Gray + "  +" + strings.Repeat("-", width) + "+" + Reset)
	for i, opt := range options {
		chars := []rune(opt)
		if len(chars) > 57 {
			opt = string(chars[:54]) + "..."
		}
		fmt.Printf("%s  |%s  %s[%02d]%s  %-57s%s |%s\n", Gray, Reset, Red, i+1, Reset, opt, Gray, Reset)
	}
	fmt.Printf("%s  |%s  %s[00]%s  %-57s%s |%s\n", Gray, Reset, Gray, Reset, exitLabel, Gray, Reset)
	fmt.Println(Gray + "  +" + strings.Repeat("-", width) + "+" + Reset)
	fmt.Println()
	for {
		fmt.Print("  " + Red + "resiletops" + Reset + " > ")
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "0" || line == "00" || strings.EqualFold(line, "q") || strings.EqualFold(line, "quit") || (err != nil && line == "") {
			return -1
		}
		n, parseErr := strconv.Atoi(line)
		if parseErr == nil && n >= 1 && n <= len(options) {
			return n - 1
		}
		fmt.Printf("%s  Choose 0–%d.%s\n", Yellow, len(options), Reset)
		if err != nil {
			return -1
		}
	}
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
	if !interactive {
		fmt.Println(label + "...")
		err := work()
		if err != nil {
			fmt.Printf("[FAIL] %s: %v\n", label, err)
		} else {
			fmt.Printf("[OK] %s\n", label)
		}
		return err
	}
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

func StepHeader(step int, total int, title string) {
	fmt.Println()
	fmt.Printf("%s%s=== STEP %d OF %d: %s ===%s\n", Bold, Red, step, total, strings.ToUpper(title), Reset)
	fmt.Println()
}

func AskConfirm(prompt string, defTrue bool) bool {
	hint := "Y/n"
	if !defTrue {
		hint = "y/N"
	}
	fmt.Printf("%s [%s]: ", prompt, hint)
	line, err := reader.ReadString(10)
	if err != nil {
		return false
	}
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return defTrue
	}
	return line == "y" || line == "yes"
}

func Info(msg string) {
	fmt.Println(Gray + "  [i] " + msg + Reset)
}

func Success(msg string) {
	fmt.Println(Green + Bold + "  [OK] " + msg + Reset)
}

func Warning(msg string) {
	fmt.Println(Yellow + "  [!] " + msg + Reset)
}

func Card(title string, lines []string) {
	fmt.Println()
	fmt.Println(Red + "  ┌──────────────────────────────────────────────────────────────┐" + Reset)
	fmt.Printf("%s  │ %s%-60s%s │%s\n", Red, Bold, title, Reset+Red, Reset)
	fmt.Println(Red + "  ├──────────────────────────────────────────────────────────────┤" + Reset)
	for _, l := range lines {
		if chars := []rune(l); len(chars) > 60 {
			l = string(chars[:57]) + "..."
		}
		fmt.Printf("%s  │%s %-60s %s│%s\n", Red, Reset, l, Red, Reset)
	}
	fmt.Println(Red + "  └──────────────────────────────────────────────────────────────┘" + Reset)
	fmt.Println()
}
