package gui

import (
	"net"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/MatthieuSarter/socquette/internal/config"
)

// proxyFields points at the fields of one proxy section in the working configuration.
type proxyFields struct {
	enabled, anonymous *bool
	listen             *string
	port               *int
}

func listenSuggestions() []string {
	addrs, _ := net.InterfaceAddrs()
	return config.ListenSuggestions(addrs)
}

func (u *ui) proxyCard(title string, f proxyFields, extra ...*widget.FormItem) fyne.CanvasObject {
	enabled := widget.NewCheck(u.s.Enabled, nil)
	enabled.SetChecked(*f.enabled)
	enabled.OnChanged = func(b bool) { *f.enabled = b; u.markDirty() }

	listen := widget.NewSelectEntry(listenSuggestions())
	listen.SetText(*f.listen)
	listen.OnChanged = func(s string) { *f.listen = strings.TrimSpace(s); u.markDirty() }

	port := widget.NewEntry()
	port.SetText(strconv.Itoa(*f.port))
	port.OnChanged = func(s string) {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			n = 0 // rejected by config.Validate on Save
		}
		*f.port = n
		u.markDirty()
	}

	anonymous := widget.NewCheck(u.s.Anonymous+"  ⚠", nil)
	anonymous.SetChecked(*f.anonymous)
	anonymous.OnChanged = func(b bool) {
		if b == *f.anonymous {
			return
		}
		if !b {
			*f.anonymous = false
			u.markDirty()
			return
		}
		u.confirm(u.s.AnonymousWarningTitle, u.s.AnonymousWarning, func(ok bool) {
			if ok {
				*f.anonymous = true
				u.markDirty()
			} else {
				anonymous.SetChecked(false)
			}
		})
	}

	items := append([]*widget.FormItem{widget.NewFormItem(u.s.Listen, listen), widget.NewFormItem(u.s.Port, port)},
		extra...)
	return widget.NewCard(title, "", container.NewVBox(enabled, widget.NewForm(items...), anonymous))
}

func (u *ui) httpCard() fyne.CanvasObject {
	h := &u.cfg.HTTP
	return u.proxyCard(u.s.HTTPProxy, proxyFields{&h.Enabled, &h.Anonymous, &h.Listen, &h.Port})
}

func (u *ui) socksCard() fyne.CanvasObject {
	s := &u.cfg.SOCKS
	advertise := widget.NewEntry()
	advertise.SetText(s.AdvertiseAddress)
	advertise.SetPlaceHolder(u.s.AdvertiseHint)
	advertise.OnChanged = func(v string) { s.AdvertiseAddress = strings.TrimSpace(v); u.markDirty() }
	return u.proxyCard(u.s.SOCKSProxy, proxyFields{&s.Enabled, &s.Anonymous, &s.Listen, &s.Port},
		widget.NewFormItem(u.s.AdvertiseAddress, advertise))
}
