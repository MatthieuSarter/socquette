package control

import (
	"errors"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/MatthieuSarter/socquette/internal/config"
)

// firewallRuleName names the Windows rules and tags the ufw ones, so a new run replaces exactly the previous rules.
const firewallRuleName = "Socquette"

// ErrNoFirewall is returned on Linux when ufw isn't installed.
var ErrNoFirewall = errors.New("ufw not found")

// FirewallRule is an inbound port to allow.
type FirewallRule struct {
	Proto string // "tcp" or "udp"
	Port  int
}

// FirewallRules lists the inbound ports the enabled proxies need: TCP for HTTP, TCP and UDP (the relay uses the
// same port number) for SOCKS5. A proxy listening on loopback only needs none.
func FirewallRules(cfg config.Config) []FirewallRule {
	var rules []FirewallRule
	open := func(listen string) bool {
		ip := net.ParseIP(listen)
		return ip == nil || !ip.IsLoopback()
	}
	if cfg.HTTP.Enabled && open(cfg.HTTP.Listen) {
		rules = append(rules, FirewallRule{"tcp", cfg.HTTP.Port})
	}
	if cfg.SOCKS.Enabled && open(cfg.SOCKS.Listen) {
		rules = append(rules, FirewallRule{"tcp", cfg.SOCKS.Port}, FirewallRule{"udp", cfg.SOCKS.Port})
	}
	return rules
}

// ufwNumbered matches a `ufw status numbered` line holding one of our rules: "[ 3] 1080/udp  ALLOW IN  Anywhere
// # Socquette".
var ufwNumbered = regexp.MustCompile(`^\[\s*(\d+)\]\s.*#\s*` + firewallRuleName + `\s*$`)

// UfwRuleNumbers returns the numbers of our rules in `ufw status numbered` output, highest first: deleting a rule
// renumbers the ones after it, so they must go from the last.
func UfwRuleNumbers(status string) []int {
	var nums []int
	for _, line := range strings.Split(status, "\n") {
		if m := ufwNumbered.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			nums = append(nums, n)
		}
	}
	slices.Sort(nums)
	slices.Reverse(nums)
	return nums
}

// configureFirewall replaces our firewall rules with those the saved configuration needs.
func configureFirewall(paths config.Paths) error {
	cfg, _, err := config.Load(paths.Config)
	if err != nil {
		return err
	}
	return applyFirewall(FirewallRules(cfg))
}
