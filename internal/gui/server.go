package gui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/MatthieuSarter/socquette/internal/control"
)

func (u *ui) pollStatus() {
	for {
		st := control.Status(u.paths)
		fyne.Do(func() {
			u.state = st
			u.updateStatus()
		})
		time.Sleep(statusPoll)
	}
}

func (u *ui) updateStatus() {
	st := u.state
	switch {
	case st.Installed && st.Running:
		u.statusLabel.SetText(u.s.ServiceRunning)
		u.startBtn.SetText(u.s.Stop)
	case st.Installed:
		u.statusLabel.SetText(u.s.ServiceStopped)
		u.startBtn.SetText(u.s.Start)
	case st.Background:
		u.statusLabel.SetText(u.s.BackgroundRunning)
		u.startBtn.SetText(u.s.Stop)
	default:
		u.statusLabel.SetText(u.s.Stopped)
		u.startBtn.SetText(u.s.Start)
	}
	if st.Installed {
		u.serviceBtn.SetText(u.s.UninstallService)
	} else {
		u.serviceBtn.SetText(u.s.InstallService)
	}
	for _, b := range []interface {
		Enable()
		Disable()
	}{u.startBtn, u.serviceBtn, u.firewallBtn} {
		if u.busy {
			b.Disable()
		} else {
			b.Enable()
		}
	}
}

// run performs a server action off the UI thread (it may wait for a UAC prompt), then refreshes the status.
func (u *ui) run(action func() error) {
	u.busy = true
	u.updateStatus()
	go func() {
		err := action()
		st := control.Status(u.paths)
		fyne.Do(func() {
			u.busy, u.state = false, st
			u.updateStatus()
			if err != nil {
				u.showError(err)
			}
		})
	}()
}

func (u *ui) toggleServer() {
	st, paths := u.state, u.paths
	switch {
	case st.Installed && st.Running:
		u.run(func() error { return control.Request(control.Stop, paths) })
	case st.Installed:
		u.run(func() error { return control.Request(control.Start, paths) })
	case st.Background:
		u.run(func() error { return control.StopBackground(paths) })
	default:
		u.run(func() error { return control.StartBackground(paths) })
	}
}

func (u *ui) toggleService() {
	paths := u.paths
	if u.state.Installed {
		u.run(func() error { return control.Request(control.Uninstall, paths) })
		return
	}
	u.run(func() error { return control.Request(control.Install, paths) })
}

// configureFirewall opens the ports of the saved configuration, so pending edits are saved first.
func (u *ui) configureFirewall() {
	if u.dirty {
		u.save()
		if u.dirty {
			return // invalid configuration or pending confirmation: save() is showing why
		}
	}
	paths, title, done := u.paths, u.s.ConfigureFirewall, u.s.FirewallDone
	u.run(func() error {
		err := control.Request(control.Firewall, paths)
		if err == nil {
			fyne.Do(func() { dialog.ShowInformation(title, done, u.win) })
		}
		return err
	})
}
