package user

import (
	"os"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/internal/redistest"
)

func TestMain(m *testing.M) {
	os.Exit(redistest.Run(m))
}

func TestFindOldFormat(t *testing.T) {
	redistest.Flush()
	conn := connections.Redis()
	defer conn.Close()
	old := `{"enable":true,"Profile":{"account":"123","email":"","line":"","lineAccessToken":"","messenger":"","telegram":"123","telegramChat":456},` +
		`"Subscribes":[{"board":"stock","keywords":["台積電"],"authors":["abc"],"articles":null,"pushSum":{"up":10,"down":0}}]}`
	if _, err := conn.Do("SET", "user:123", old); err != nil {
		t.Fatal(err)
	}

	u, err := Find("123")
	if err != nil {
		t.Fatal(err)
	}
	if u.ChatID() != 456 || len(u.Subscribes) != 1 || u.Subscribes[0].Keywords[0] != "台積電" {
		t.Errorf("Find() = %+v", u)
	}
}

func TestFindNotFound(t *testing.T) {
	redistest.Flush()
	if _, err := Find("404"); err != ErrNotFound {
		t.Errorf("Find() err = %v, want ErrNotFound", err)
	}
}

func TestRegisterAndUpdate(t *testing.T) {
	redistest.Flush()
	if changed, err := Register("1", 1, "小明"); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, _ := Register("1", 1, "小明"); changed {
		t.Error("nothing changed")
	}
	err := Update("1", func(u *User) error {
		u.Subscribes.AddKeywords("stock", []string{"a"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := Find("1")
	if err != nil {
		t.Fatal(err)
	}
	if !u.Enable || u.Profile.Account != "1" || u.ChatID() != 1 || len(u.Subscribes) != 1 || u.DisplayName() != "小明" {
		t.Errorf("user = %+v", u)
	}
	accounts, err := Accounts()
	if err != nil || len(accounts) != 1 || accounts[0] != "1" {
		t.Errorf("Accounts() = %v, %v", accounts, err)
	}
}

func TestDisableAndRegisterAgain(t *testing.T) {
	redistest.Flush()
	Register("1", 1, "")
	if err := Disable("1"); err != nil {
		t.Fatal(err)
	}
	if u, _ := Find("1"); u.Enable {
		t.Error("user should be disabled")
	}
	if changed, _ := Register("1", 1, ""); !changed {
		t.Error("register should enable user again")
	}
	if u, _ := Find("1"); !u.Enable || u.DisplayName() != "1" {
		t.Errorf("user = %+v", u)
	}
}

func TestUpdateNotRegistered(t *testing.T) {
	redistest.Flush()
	err := Update("404", func(u *User) error { return nil })
	if err != ErrNotFound {
		t.Errorf("Update() err = %v, want ErrNotFound", err)
	}
}

func TestChatIDFallsBackToAccount(t *testing.T) {
	u := User{Profile: Profile{Account: "789"}}
	if u.ChatID() != 789 {
		t.Errorf("ChatID() = %d", u.ChatID())
	}
}
