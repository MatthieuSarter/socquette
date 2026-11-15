package control

import (
	"reflect"
	"testing"

	"github.com/MatthieuSarter/socquette/internal/config"
)

func TestFirewallRules(t *testing.T) {
	cfg := config.Defaults()
	cfg.HTTP.Enabled, cfg.SOCKS.Enabled = true, true
	want := []FirewallRule{{"tcp", 8080}, {"tcp", 1080}, {"udp", 1080}}
	if got := FirewallRules(cfg); !reflect.DeepEqual(got, want) {
		t.Errorf("both enabled: %v, want %v", got, want)
	}
	cfg.HTTP.Listen, cfg.SOCKS.Enabled = "127.0.0.1", false
	if got := FirewallRules(cfg); len(got) != 0 {
		t.Errorf("loopback HTTP, SOCKS disabled: %v, want none", got)
	}
}

func TestUfwRuleNumbers(t *testing.T) {
	status := "Status: active\r\n\n     To                         Action      From\n" +
		"[ 1] 22/tcp                     ALLOW IN    Anywhere\n" +
		"[ 2] 8080/tcp                   ALLOW IN    Anywhere                   # Socquette\n" +
		"[ 3] 1080/udp                   ALLOW IN    Anywhere                   # other\n" +
		"[10] 1080/tcp (v6)              ALLOW IN    Anywhere (v6)              # Socquette\n"
	if got := UfwRuleNumbers(status); !reflect.DeepEqual(got, []int{10, 2}) {
		t.Errorf("UfwRuleNumbers = %v, want [10 2]", got)
	}
}
