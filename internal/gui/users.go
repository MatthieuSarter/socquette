package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/MatthieuSarter/socquette/internal/auth"
	"github.com/MatthieuSarter/socquette/internal/config"
)

func (u *ui) usersCard() fyne.CanvasObject {
	u.usersBox = container.NewVBox()
	u.refreshUsers()
	add := widget.NewButton(u.s.AddUser, u.addUser)
	return widget.NewCard(u.s.Users, "", container.NewVBox(u.usersBox, container.NewHBox(add)))
}

// setRight changes one right of a user, found by name since the slice is replaced on every add/remove.
func (u *ui) setRight(name string, set func(*config.User)) {
	for i := range u.cfg.Users {
		if u.cfg.Users[i].Name == name {
			set(&u.cfg.Users[i])
			u.markDirty()
		}
	}
}

func (u *ui) refreshUsers() {
	u.usersBox.RemoveAll()
	if len(u.cfg.Users) == 0 {
		u.usersBox.Add(widget.NewLabel(u.s.NoUsers))
		return
	}
	bold := fyne.TextStyle{Bold: true}
	grid := container.NewGridWithColumns(5,
		widget.NewLabelWithStyle(u.s.Name, fyne.TextAlignLeading, bold),
		widget.NewLabelWithStyle("HTTP", fyne.TextAlignLeading, bold),
		widget.NewLabelWithStyle("SOCKS", fyne.TextAlignLeading, bold),
		widget.NewLabel(""), widget.NewLabel(""))
	for _, user := range u.cfg.Users {
		name := user.Name
		httpCheck := widget.NewCheck("", func(b bool) { u.setRight(name, func(x *config.User) { x.HTTP = b }) })
		httpCheck.Checked = user.HTTP
		socksCheck := widget.NewCheck("", func(b bool) { u.setRight(name, func(x *config.User) { x.SOCKS = b }) })
		socksCheck.Checked = user.SOCKS
		label := widget.NewLabel(name)
		label.Truncation = fyne.TextTruncateEllipsis
		grid.Add(label)
		grid.Add(httpCheck)
		grid.Add(socksCheck)
		grid.Add(widget.NewButton(u.s.ChangePassword, func() { u.changePassword(name) }))
		grid.Add(widget.NewButton(u.s.Delete, func() { u.deleteUser(name) }))
	}
	u.usersBox.Add(grid)
}

func (u *ui) addUser() {
	name := widget.NewEntry()
	pw1, pw2 := widget.NewPasswordEntry(), widget.NewPasswordEntry()
	httpCheck, socksCheck := widget.NewCheck("HTTP", nil), widget.NewCheck("SOCKS", nil)
	httpCheck.SetChecked(true)
	socksCheck.SetChecked(true)
	items := []*widget.FormItem{
		widget.NewFormItem(u.s.Name, name),
		widget.NewFormItem(u.s.Password, pw1),
		widget.NewFormItem(u.s.ConfirmPassword, pw2),
		widget.NewFormItem("", container.NewHBox(httpCheck, socksCheck)),
	}
	u.form(u.s.AddUser, items, func() error {
		c, err := config.AddUser(u.cfg, name.Text, pw1.Text, pw2.Text, httpCheck.Checked, socksCheck.Checked,
			auth.HashPassword)
		if err == nil {
			u.cfg = c
		}
		return err
	})
}

func (u *ui) changePassword(name string) {
	pw1, pw2 := widget.NewPasswordEntry(), widget.NewPasswordEntry()
	items := []*widget.FormItem{
		widget.NewFormItem(u.s.Password, pw1),
		widget.NewFormItem(u.s.ConfirmPassword, pw2),
	}
	u.form(u.s.ChangePassword+" — "+name, items, func() error {
		c, err := config.SetPassword(u.cfg, name, pw1.Text, pw2.Text, auth.HashPassword)
		if err == nil {
			u.cfg = c
		}
		return err
	})
}

// form shows a dialog; on OK, apply runs and either refreshes the users and networks or shows its error.
func (u *ui) form(title string, items []*widget.FormItem, apply func() error) {
	d := dialog.NewForm(title, u.s.OK, u.s.Cancel, items, func(ok bool) {
		if !ok {
			return
		}
		if err := apply(); err != nil {
			u.showError(err)
			return
		}
		u.refreshUsers()
		u.refreshNetworks()
		u.markDirty()
	}, u.win)
	d.Resize(fyne.NewSize(420, 0))
	d.Show()
}

func (u *ui) deleteUser(name string) {
	u.confirm(u.s.Delete, fmt.Sprintf(u.s.DeleteUserF, name), func(ok bool) {
		if ok {
			u.cfg = config.RemoveUser(u.cfg, name)
			u.refreshUsers()
			u.markDirty()
		}
	})
}
