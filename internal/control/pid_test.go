package control

import "testing"

func TestParsePID(t *testing.T) {
	if pid, err := ParsePID([]byte("1234\n")); err != nil || pid != 1234 {
		t.Errorf("ParsePID = %d, %v", pid, err)
	}
	for _, bad := range []string{"", "abc", "-1", "0"} {
		if _, err := ParsePID([]byte(bad)); err == nil {
			t.Errorf("ParsePID(%q) accepted", bad)
		}
	}
}

func TestServiceConfig(t *testing.T) {
	c := ServiceConfig(`C:\socquette`)
	if c.Name != "socquette" || c.Arguments[0] != "--serve" || c.EnvVars["SOCQUETTE_CONF"] != `C:\socquette` {
		t.Errorf("config = %+v", c)
	}
}
