// Command exp112-pl-patch installs the Polish localisation of eXperience112
// (from the user's own Polish retail disc or ISO) into the Steam release.
//
// Usage:
//
//	exp112-pl-patch [-game DIR] [-source PATH] [-restore] [-nogfx] [-yes]
//
// The program contains no game data; everything is read from the user's
// copies. Its interface is in Polish.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Nigatsu/experience112-pl-steam/internal/patch"
	"github.com/Nigatsu/experience112-pl-steam/internal/platform"
)

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	platform.InitConsole()
	var (
		game    = flag.String("game", "", `folder gry w wersji Steam (…\steamapps\common\Experience112)`)
		source  = flag.String("source", "", "polski obraz .iso, płyta (folder z data1.hdr) lub folder zainstalowanej polskiej wersji")
		restore = flag.Bool("restore", false, "przywróć oryginalne pliki gry (cofnij spolszczenie)")
		noGfx   = flag.Bool("nogfx", false, "pomiń poprawkę ucinania liter (g, j, y)")
		yes     = flag.Bool("yes", false, "nie zadawaj pytań (tryb automatyczny)")
	)
	flag.Parse()

	ui := &console{in: bufio.NewReader(os.Stdin), out: os.Stdout, auto: *yes}
	ui.Printf("eXperience112 — spolszczenie wersji Steam  %s\n%s\n", version, strings.Repeat("=", 60))

	err := execute(ui, *game, *source, *restore, *noGfx)
	if err != nil {
		ui.Printf("\nBŁĄD: %v\n", err)
		var ee *patch.EditionError
		if errors.As(err, &ee) {
			ui.Printf("%s\n", patch.SwitchToEnglishHelp)
		}
	}
	if !*yes {
		ui.Printf("\nNaciśnij Enter, aby zakończyć...")
		ui.in.ReadString('\n')
	}
	if err != nil {
		return 1
	}
	return 0
}

func execute(ui *console, gameFlag, sourceFlag string, restore, noGfx bool) error {
	game, err := patch.FindGame(gameFlag, ui)
	if err != nil {
		return err
	}
	ui.Printf("Gra (Steam): %s\n", game)
	if restore {
		return patch.Restore(game, ui)
	}
	if err := patch.CheckEdition(game, ui); err != nil {
		return err
	}
	src, err := patch.FindSource(sourceFlag, ui)
	if err != nil {
		return err
	}
	defer src.Close()
	ui.Printf("Źródło (PL): %s\n\n", src.Desc)
	if !ui.Confirm("Zainstalować polską wersję językową?", true) {
		return patch.ErrAborted
	}
	if err := patch.Install(patch.Options{Game: game, Source: src, NoTextFix: noGfx}, ui); err != nil {
		return err
	}
	ui.Printf("\nGotowe! Uruchom grę ze Steam.\n" +
		"Uwaga: „Sprawdź spójność plików” lub zmiana języka w Steam przywraca\n" +
		"oryginalne pliki — wtedy uruchom ten program ponownie.\n")
	return nil
}

// console implements patch.UI on a terminal.
type console struct {
	in   *bufio.Reader
	out  io.Writer
	auto bool // answer every question with its default
}

func (c *console) Printf(format string, args ...any) { fmt.Fprintf(c.out, format, args...) }

func (c *console) Confirm(question string, def bool) bool {
	if c.auto {
		return def
	}
	opts := " [T/n] "
	if !def {
		opts = " [t/N] "
	}
	c.Printf("%s%s", question, opts)
	s, err := c.in.ReadString('\n')
	if errors.Is(err, io.EOF) && s == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return def
	case "t", "tak", "y", "yes":
		return true
	}
	return false
}

func (c *console) Prompt(question string) string {
	if c.auto {
		return ""
	}
	c.Printf("%s\n> ", question)
	s, _ := c.in.ReadString('\n')
	return strings.Trim(strings.TrimSpace(s), `"`)
}
