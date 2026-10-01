package tui

import (
	"bufio"
	"strings"
	"testing"
)

func TestMenuRetriesAndEOF(t *testing.T) {
	original := reader
	defer func() { reader = original }()
	reader = bufio.NewReader(strings.NewReader("invalid\n2\n"))
	if got := Menu("test", []string{"one", "two"}); got != 1 {
		t.Fatalf("selection=%d", got)
	}
	reader = bufio.NewReader(strings.NewReader("00\n"))
	if got := MainMenu([]string{"one"}); got != -1 {
		t.Fatalf("00 did not exit: %d", got)
	}
	reader = bufio.NewReader(strings.NewReader(""))
	if got := Menu("test", []string{"one"}); got != -1 {
		t.Fatalf("EOF selection=%d", got)
	}
	if AskConfirm("confirm", true) {
		t.Fatal("EOF must not confirm")
	}
}
