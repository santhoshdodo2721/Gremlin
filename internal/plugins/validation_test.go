package plugins

import "testing"

func TestValidateParams(t *testing.T) {
	for _, params := range []Params{{"duration_s": "0"}, {"duration_s": "oops"}, {"percent": "NaN"}, {"percent": "101"}, {"quota_pct": "-1"}, {"method": "bad"}, {"restart": "yes"}} {
		if err := ValidateParams("cpu", "target", params); err == nil {
			t.Errorf("accepted %v", params)
		}
	}
	if err := ValidateParams("cpu", "target", Params{"duration_s": "10", "quota_pct": "5", "method": "throttle"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateParams("cpu", "", nil); err == nil {
		t.Fatal("accepted empty target")
	}
}
