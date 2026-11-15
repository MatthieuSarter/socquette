package i18n

import (
	"reflect"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := map[string]Lang{
		"fr-FR": FR, "fr": FR, "FR_ca": FR, "es-ES": ES, "es": ES, "en-US": EN, "de-DE": EN, "": EN,
	}
	for locale, want := range tests {
		if got := Detect(locale); got != want {
			t.Errorf("Detect(%q) = %q, want %q", locale, got, want)
		}
	}
}

func TestAllStringsTranslated(t *testing.T) {
	for _, l := range All {
		v := reflect.ValueOf(For(l))
		for i := range v.NumField() {
			if v.Field(i).String() == "" {
				t.Errorf("%s: %s is empty", l, v.Type().Field(i).Name)
			}
		}
	}
}

func TestForUnknownIsEnglish(t *testing.T) {
	if For("xx").Save != For(EN).Save {
		t.Error("unknown language is not English")
	}
}
