// Command tuiplay is a terminal music player. It streams from a Navidrome
// server through the Subsonic API. The look follows ncmpcpp.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"git.hemmalab.se/scuttle/tuiplay/internal/config"
	"git.hemmalab.se/scuttle/tuiplay/internal/control"
	"git.hemmalab.se/scuttle/tuiplay/internal/player"
	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"
	"git.hemmalab.se/scuttle/tuiplay/internal/ui"
	"git.hemmalab.se/scuttle/tuiplay/internal/version"
)

func main() {
	// The "ctl" subcommand sends a control command to a running tuiplay
	// and exits. It runs before flag parsing so command arguments like
	// ">" or "-10" are not read as flags.
	if len(os.Args) > 1 && os.Args[1] == "ctl" {
		runCtl(os.Args[2:])
		return
	}

	configPath := flag.String("config", "", "path to the config file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("tuiplay", version.Version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		if errors.Is(err, config.ErrCreatedTemplate) {
			path, _ := config.DefaultPath()
			if *configPath != "" {
				path = *configPath
			}
			fmt.Println("No config file was found.")
			fmt.Println("A template was created at:", path)
			fmt.Println("Edit the server URL and credentials, then run tuiplay again.")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	client := subsonic.New(cfg.ServerURL, cfg.Username, cfg.Password, cfg.MaxBitRate)

	keys, err := config.ResolveKeys(cfg.Keys)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := client.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, "cannot reach server:", err)
		os.Exit(1)
	}

	pl, err := player.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "audio error:", err)
		os.Exit(1)
	}

	state := ui.UIState{
		RightShown: cfg.UI.ActiveView != "hidden",
		SplitRatio: cfg.UI.SplitRatio,
	}
	cacheDir, err := cfg.LyricsCacheDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not resolve lyrics cache dir:", err)
		cacheDir = ""
	}
	theme, err := ui.ResolveTheme(cfg.Theme, cfg.ThemeColors)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	set := ui.Settings{
		SeekSeconds:           cfg.SeekSeconds,
		CrossfadeSeconds:      cfg.CrossfadeSeconds,
		RadioQueueSize:        cfg.RadioSize(),
		NSPPath:               cfg.NSPPath,
		LrclibDBPath:          cfg.LrclibDBPath,
		LyricsCacheDir:        cacheDir,
		LyricsMissRecheckDays: cfg.LyricsRecheckDays(),
		LyricsProviders:       cfg.ResolvedLyricsProviders(),
		ControlSocket:         cfg.ControlSocketPath(),
		Theme:                 theme,
	}

	final, err := ui.Run(client, pl, state, set, keys)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ui error:", err)
		os.Exit(1)
	}

	// Persist the interface state. A save error is not fatal.
	activeView := "nav"
	if !final.RightShown {
		activeView = "hidden"
	}
	if serr := config.SaveUIState(*configPath, config.UIState{
		ActiveView: activeView,
		SplitRatio: final.SplitRatio,
	}); serr != nil {
		fmt.Fprintln(os.Stderr, "warning: could not save UI state:", serr)
	}
}

// runCtl sends one control command to a running tuiplay over the control
// socket, prints the reply, and exits. Usage: tuiplay ctl <command> [args].
// It reads the socket path from the config file. An optional
// "--config <path>" before the command overrides the config path.
func runCtl(args []string) {
	configPath := ""
	// Accept an optional leading --config/-config flag.
	for len(args) >= 2 && (args[0] == "--config" || args[0] == "-config") {
		configPath = args[1]
		args = args[2:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: tuiplay ctl <command> [args]")
		fmt.Fprintln(os.Stderr, "commands: play pause stop next prev seek <sec> volume <+|-|N> playlist <name>")
		os.Exit(2)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		if errors.Is(err, config.ErrCreatedTemplate) {
			fmt.Fprintln(os.Stderr, "no config file yet; run tuiplay once and set control_socket")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	sock := cfg.ControlSocketPath()
	if sock == "" {
		fmt.Fprintln(os.Stderr, "control is off: set control_socket in the config")
		os.Exit(1)
	}

	line := strings.Join(args, " ")
	reply, err := control.Send(sock, line)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctl error:", err)
		os.Exit(1)
	}
	fmt.Println(reply)
	if strings.HasPrefix(reply, "err:") {
		os.Exit(1)
	}
}
