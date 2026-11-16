package gui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/MatthieuSarter/socquette/internal/config"
)

func (u *ui) networksCard() fyne.CanvasObject {
	u.networksBox = container.NewVBox()
	u.refreshNetworks()
	add := widget.NewButton(u.s.AddNetwork, u.addNetwork)
	return widget.NewCard(u.s.Networks, u.s.NetworksHint, container.NewVBox(u.networksBox, container.NewHBox(add)))
}

// setNetworkRight changes one right of a network, found by name since the slice is replaced on every add/remove.
func (u *ui) setNetworkRight(name string, set func(*config.Network)) {
	for i := range u.cfg.Networks {
		if u.cfg.Networks[i].Name == name {
			set(&u.cfg.Networks[i])
			u.markDirty()
		}
	}
}

func (u *ui) refreshNetworks() {
	u.networksBox.RemoveAll()
	if len(u.cfg.Networks) == 0 {
		u.networksBox.Add(widget.NewLabel(u.s.NoNetworks))
		return
	}
	bold := fyne.TextStyle{Bold: true}
	grid := container.NewGridWithColumns(6,
		widget.NewLabelWithStyle(u.s.Name, fyne.TextAlignLeading, bold),
		widget.NewLabelWithStyle(u.s.Addresses, fyne.TextAlignLeading, bold),
		widget.NewLabelWithStyle("HTTP", fyne.TextAlignLeading, bold),
		widget.NewLabelWithStyle("SOCKS", fyne.TextAlignLeading, bold),
		widget.NewLabel(""), widget.NewLabel(""))
	for _, n := range u.cfg.Networks {
		name, addrs := n.Name, strings.Join(n.Addresses, ", ")
		httpCheck := widget.NewCheck("", func(b bool) {
			u.setNetworkRight(name, func(x *config.Network) { x.HTTP = b })
		})
		httpCheck.Checked = n.HTTP
		socksCheck := widget.NewCheck("", func(b bool) {
			u.setNetworkRight(name, func(x *config.Network) { x.SOCKS = b })
		})
		socksCheck.Checked = n.SOCKS
		label, addrLabel := widget.NewLabel(name), widget.NewLabel(addrs)
		label.Truncation, addrLabel.Truncation = fyne.TextTruncateEllipsis, fyne.TextTruncateEllipsis
		grid.Add(label)
		grid.Add(addrLabel)
		grid.Add(httpCheck)
		grid.Add(socksCheck)
		grid.Add(widget.NewButton(u.s.EditAddresses, func() { u.editAddresses(name, addrs) }))
		grid.Add(widget.NewButton(u.s.Delete, func() { u.deleteNetwork(name) }))
	}
	u.networksBox.Add(grid)
}

func (u *ui) addressesEntry(text string) *widget.Entry {
	e := widget.NewMultiLineEntry()
	e.SetText(text)
	e.SetPlaceHolder(u.s.AddressesHint)
	e.Wrapping = fyne.TextWrapWord
	return e
}

func (u *ui) addNetwork() {
	name, addrs := widget.NewEntry(), u.addressesEntry("")
	httpCheck, socksCheck := widget.NewCheck("HTTP", nil), widget.NewCheck("SOCKS", nil)
	httpCheck.SetChecked(true)
	socksCheck.SetChecked(true)
	items := []*widget.FormItem{
		widget.NewFormItem(u.s.Name, name),
		widget.NewFormItem(u.s.Addresses, addrs),
		widget.NewFormItem("", container.NewHBox(httpCheck, socksCheck)),
	}
	u.form(u.s.AddNetwork, items, func() error {
		c, err := config.AddNetwork(u.cfg, name.Text, addrs.Text, httpCheck.Checked, socksCheck.Checked)
		if err == nil {
			u.cfg = c
		}
		return err
	})
}

func (u *ui) editAddresses(name, current string) {
	addrs := u.addressesEntry(current)
	u.form(u.s.EditAddresses+" — "+name, []*widget.FormItem{widget.NewFormItem(u.s.Addresses, addrs)}, func() error {
		c, err := config.SetNetworkAddresses(u.cfg, name, addrs.Text)
		if err == nil {
			u.cfg = c
		}
		return err
	})
}

func (u *ui) deleteNetwork(name string) {
	u.confirm(u.s.Delete, fmt.Sprintf(u.s.DeleteNetworkF, name), func(ok bool) {
		if ok {
			u.cfg = config.RemoveNetwork(u.cfg, name)
			u.refreshNetworks()
			u.markDirty()
		}
	})
}
