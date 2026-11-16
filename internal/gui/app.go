// Package gui is the Fyne configuration window: it edits socquette.toml and controls the server.
package gui

import (
	"errors"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/MatthieuSarter/socquette/internal/assets"
	"github.com/MatthieuSarter/socquette/internal/config"
	"github.com/MatthieuSarter/socquette/internal/control"
	"github.com/MatthieuSarter/socquette/internal/i18n"
)

const (
	appID      = "io.github.matthieusarter.socquette"
	prefLang   = "lang"
	prefTheme  = "theme"
	statusPoll = 2 * time.Second
)

type ui struct {
	app   fyne.App
	win   fyne.Window
	paths config.Paths
	lang  i18n.Lang
	s     i18n.Strings

	cfg     config.Config // working copy, written on Save
	dirty   bool
	loadErr error // the file was unreadable at start: saving would replace it with the defaults-based copy

	dirtyLabel  *widget.Label
	statusLabel *widget.Label
	startBtn    *widget.Button
	serviceBtn  *widget.Button
	firewallBtn *widget.Button
	usersBox    *fyne.Container
	networksBox *fyne.Container
	state       control.State
	busy        bool
}

// Run opens the configuration window and returns when it is closed.
func Run(paths config.Paths) {
	a := app.NewWithID(appID)
	a.SetIcon(fyne.NewStaticResource("icon.png", assets.Icon))
	u := &ui{app: a, paths: paths, win: a.NewWindow("Socquette")}
	cfg, _, loadErr := config.Load(paths.Config)
	if loadErr != nil {
		cfg = config.Defaults()
	}
	u.cfg, u.loadErr = cfg, loadErr
	applyTheme(a, a.Preferences().StringWithFallback(prefTheme, themeAuto))
	detected := i18n.Detect(string(lang.SystemLocale()))
	u.setLang(i18n.Lang(a.Preferences().StringWithFallback(prefLang, string(detected))))
	u.win.Resize(fyne.NewSize(860, 620))
	u.win.SetCloseIntercept(u.close)
	if loadErr != nil {
		u.showError(loadErr)
	}
	go u.pollStatus()
	u.win.ShowAndRun()
}

func (u *ui) setLang(l i18n.Lang) {
	u.lang, u.s = l, i18n.For(l)
	u.app.Preferences().SetString(prefLang, string(l))
	u.win.SetContent(u.build())
	u.updateStatus()
}

func (u *ui) build() fyne.CanvasObject {
	top := container.NewHBox(layout.NewSpacer(),
		widget.NewLabel(u.s.Language), u.langSelect(), widget.NewLabel(u.s.Theme), u.themeSelect())

	u.statusLabel = widget.NewLabel("")
	u.startBtn = widget.NewButton("", u.toggleServer)
	u.serviceBtn = widget.NewButton("", u.toggleService)
	u.firewallBtn = widget.NewButton(u.s.ConfigureFirewall, u.configureFirewall)
	status := container.NewHBox(widget.NewLabelWithStyle(u.s.Server, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		u.statusLabel, layout.NewSpacer(), u.startBtn, u.serviceBtn, u.firewallBtn)

	proxies := container.NewGridWithColumns(2, u.httpCard(), u.socksCard())

	u.dirtyLabel = widget.NewLabel("")
	if u.dirty {
		u.dirtyLabel.SetText(u.s.Unsaved)
	}
	save := widget.NewButton(u.s.Save, u.save)
	save.Importance = widget.HighImportance
	path := widget.NewLabel(strings.ReplaceAll(u.s.ConfigFileF, "%s", u.paths.Config))
	path.Truncation = fyne.TextTruncateEllipsis
	bottom := container.NewBorder(nil, nil, nil, container.NewHBox(u.dirtyLabel, save), path)

	body := container.NewVScroll(container.NewVBox(proxies, u.usersCard(), u.networksCard()))
	return container.NewBorder(container.NewVBox(top, widget.NewSeparator(), status), bottom, nil, nil, body)
}

func (u *ui) langSelect() *widget.Select {
	names := make([]string, len(i18n.All))
	for i, l := range i18n.All {
		names[i] = i18n.For(l).LanguageName
	}
	sel := widget.NewSelect(names, nil)
	sel.SetSelected(u.s.LanguageName)
	sel.OnChanged = func(name string) {
		for _, l := range i18n.All {
			if i18n.For(l).LanguageName == name && l != u.lang {
				// Rebuild after this callback returns: the select being clicked is replaced.
				fyne.Do(func() { u.setLang(l) })
			}
		}
	}
	return sel
}

func (u *ui) themeSelect() *widget.Select {
	modes := []string{themeAuto, themeLight, themeDark}
	labels := []string{u.s.ThemeAuto, u.s.ThemeLight, u.s.ThemeDark}
	sel := widget.NewSelect(labels, nil)
	current := u.app.Preferences().StringWithFallback(prefTheme, themeAuto)
	for i, m := range modes {
		if m == current {
			sel.SetSelected(labels[i])
		}
	}
	sel.OnChanged = func(label string) {
		for i, l := range labels {
			if l == label {
				u.app.Preferences().SetString(prefTheme, modes[i])
				applyTheme(u.app, modes[i])
			}
		}
	}
	return sel
}

func (u *ui) markDirty() {
	u.dirty = true
	u.dirtyLabel.SetText(u.s.Unsaved)
}

func (u *ui) save() {
	if u.loadErr != nil {
		u.confirm(u.s.Save, u.s.OverwriteInvalid, func(ok bool) {
			if ok {
				u.loadErr = nil
				u.save()
			}
		})
		return
	}
	if err := config.Save(u.paths.Config, u.cfg); err != nil {
		u.showError(err)
		return
	}
	u.dirty = false
	u.dirtyLabel.SetText(u.s.Saved)
}

func (u *ui) close() {
	if !u.dirty {
		u.win.Close()
		return
	}
	u.confirm(u.s.Unsaved, u.s.QuitUnsaved, func(ok bool) {
		if ok {
			u.win.Close()
		}
	})
}

// errText translates the errors the UI knows; the others are shown as they are.
func (u *ui) errText(err error) string {
	known := []struct {
		err error
		msg string
	}{
		{config.ErrNameEmpty, u.s.ErrNameEmpty},
		{config.ErrNameTaken, u.s.ErrNameTaken},
		{config.ErrPasswordEmpty, u.s.ErrPasswordEmpty},
		{config.ErrPasswordMismatch, u.s.ErrPasswordMismatch},
		{config.ErrInvalidPort, u.s.ErrInvalidPort},
		{config.ErrInvalidListen, u.s.ErrInvalidListen},
		{config.ErrInvalidAdvertise, u.s.ErrInvalidAdvertise},
		{config.ErrSameAddress, u.s.ErrSameAddress},
		{config.ErrInvalidNetwork, u.s.ErrInvalidNetwork},
		{config.ErrNoAddresses, u.s.ErrNoAddresses},
		{control.ErrCancelled, u.s.ElevationCancelled},
		{control.ErrNoFirewall, u.s.ErrNoFirewall},
	}
	var lines []string
	for _, k := range known {
		if errors.Is(err, k.err) {
			lines = append(lines, k.msg)
		}
	}
	if len(lines) == 0 {
		return u.s.ErrOther + "\n" + err.Error()
	}
	return strings.Join(lines, "\n")
}

func (u *ui) showError(err error) {
	label := widget.NewLabel(u.errText(err))
	label.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustom(u.s.Error, u.s.OK, label, u.win)
	d.Resize(fyne.NewSize(460, 0))
	d.Show()
}

func (u *ui) confirm(title, message string, done func(bool)) {
	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm(title, u.s.Yes, u.s.No, label, done, u.win)
	d.Resize(fyne.NewSize(460, 0))
	d.Show()
}
