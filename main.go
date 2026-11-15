// Socquette is an authenticated SOCKS5 and HTTP proxy with a desktop configuration window.
//
// Without argument it opens the configuration window; with --serve it runs the proxy server (what the service runs).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/MatthieuSarter/socquette/internal/config"
	"github.com/MatthieuSarter/socquette/internal/control"
	"github.com/MatthieuSarter/socquette/internal/gui"
	"github.com/MatthieuSarter/socquette/internal/server"
	"github.com/MatthieuSarter/socquette/internal/version"
)

func main() {
	if len(os.Args) > 1 {
		attachConsole()
	}
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("socquette", flag.ContinueOnError)
	serve := fs.Bool("serve", false, "")
	showVersion := fs.Bool("version", false, "")
	// Hidden: used by the configuration window to run a service action with elevated rights.
	action := fs.String("service", "", "")
	conf := fs.String("conf", "", "")
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), "usage: socquette [--serve | --version]\n\n"+
			"  (no argument)  open the configuration window\n"+
			"  --serve        run the proxy server\n"+
			"  --version      print the version\n\n"+
			"The configuration is socquette.toml, in $SOCQUETTE_CONF or else the current directory.\n")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}
	cwd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	dir := os.Getenv("SOCQUETTE_CONF")
	if *conf != "" {
		dir = *conf
	}
	paths := config.ResolvePaths(dir, cwd)

	switch {
	case *showVersion:
		fmt.Println("socquette", version.Version)
	case *action != "":
		if err := control.Do(control.Action(*action), paths); err != nil {
			if log, closer, lerr := server.NewLogger(paths.Log, os.Stderr); lerr == nil {
				log.Error("service action failed", "action", *action, "error", err)
				_ = closer.Close()
			}
			return 1
		}
	case *serve:
		if err := control.RunServe(paths); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			return 1
		}
	default:
		gui.Run(paths)
	}
	return 0
}
