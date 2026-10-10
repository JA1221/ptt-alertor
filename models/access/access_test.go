package access

import (
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/internal/redistest"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

func TestMain(m *testing.M) {
	os.Exit(redistest.Run(m))
}

func TestMembership(t *testing.T) {
	redistest.Flush()
	if added, err := Request("1"); err != nil || !added {
		t.Fatal(added, err)
	}
	if added, _ := Request("1"); added {
		t.Error("second request should not be added")
	}
	if ok, _ := IsPending("1"); !ok {
		t.Error("1 should be pending")
	}
	if err := Approve("1"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := IsMember("1"); !ok {
		t.Error("1 should be member")
	}
	if ok, _ := IsPending("1"); ok {
		t.Error("1 should not be pending")
	}

	if err := AddAdmin("2"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := IsMember("2"); !ok {
		t.Error("admin should be member")
	}
	members, _ := Members()
	sort.Strings(members)
	if !reflect.DeepEqual(members, []string{"1", "2"}) {
		t.Errorf("Members() = %v", members)
	}

	if err := Remove("2"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := IsAdmin("2"); ok {
		t.Error("removed account should not be admin")
	}
}

func TestClaim(t *testing.T) {
	redistest.Flush()
	code, err := ClaimCode()
	if err != nil || len(code) != 16 {
		t.Fatal(code, err)
	}
	if again, _ := ClaimCode(); again != code {
		t.Error("claim code should be kept until used")
	}
	if ok, _ := Claim("1", "wrong"); ok {
		t.Error("wrong code should not claim")
	}
	if ok, err := Claim("1", code); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, _ := IsAdmin("1"); !ok {
		t.Error("1 should be admin")
	}
	if ok, _ := Claim("2", code); ok {
		t.Error("code is used once")
	}
}

func TestSettings(t *testing.T) {
	redistest.Flush()
	if mode, _ := Registration(); mode != ModeApproval {
		t.Errorf("default mode = %s", mode)
	}
	SetRegistration(ModeOpen)
	if mode, _ := Registration(); mode != ModeOpen {
		t.Errorf("mode = %s", mode)
	}
	if token, _ := Token(); token != "" {
		t.Errorf("token = %q", token)
	}
	SetToken("abc")
	if token, _ := Token(); token != "abc" {
		t.Errorf("token = %q", token)
	}
}

func TestMigrate(t *testing.T) {
	redistest.Flush()
	user.Register("1", 1, "")
	user.Register("2", 2, "")
	user.Disable("2")
	if err := Migrate(); err != nil {
		t.Fatal(err)
	}
	if members, _ := Members(); !reflect.DeepEqual(members, []string{"1"}) {
		t.Errorf("Members() = %v", members)
	}
	// only once
	user.Register("3", 3, "")
	Migrate()
	if members, _ := Members(); len(members) != 1 {
		t.Errorf("Members() = %v", members)
	}
}
