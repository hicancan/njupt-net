package zfw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestSelfOverviewAndProfileExposeDisplayedFields(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Self/dashboard":
			fmt.Fprint(w, selfTestPage(`<div class="panel-body"><div class="user-info1"><dl><dt>12 <small>分钟</small></dt><dd>已用时长</dd></dl></div>
<div class="row"><div><label>账　　号：</label></div><div>fixture-account</div></div>
<div class="row"><label>状　　态：</label><div><span>正常</span></div></div><label>套餐：</label><div>套餐</div><label>计费方式：</label><div>包月</div><label>计费周期：</label><div>自然月</div><label>账户余额：</label><div>0 元</div><label>可用时长：</label><div>不限</div><label>消费保护：</label><div>不限</div></div>`, "fixture-account"))
		case "/Self/setting/personList":
			fmt.Fprint(w, selfTestPage(`<div id="userInfo"><form action="/Self/setting/updateUserSecurity"><div><label>用户类别：</label><div><p>学生</p></div><label>失效日期：</label><div><p>2027-01-01</p></div></div></form></div>`, "fixture-account"))
		}
	})
	account, err := s.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if account.Account != "fixture-account" || account.Status != "正常" || account.UsedTime != "12 分钟" {
		t.Fatalf("incorrect displayed fields: %v", account)
	}
	profile, err := s.Profile(context.Background())
	if err != nil || len(profile.Fields) != 2 || !profile.ReadOnly || profile.Fields[0].Value != "学生" || profile.Fields[1].Value != "2027-01-01" {
		t.Fatalf("profile = %v, %v", profile, err)
	}
	encoded, _ := json.Marshal(account)
	if strings.Contains(string(encoded), "server-secret") {
		t.Error("account exposed the embedded password")
	}
}

func TestProfileDoesNotSilentlyIgnoreAnEditableForm(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, selfTestPage(`<div id="userInfo"><form action="/Self/setting/updateUserSecurity"><label>用户类别：</label><div>学生</div><textarea name="phone"></textarea></form></div>`, "fixture-account"))
	})
	if _, err := s.Profile(context.Background()); err == nil || !strings.Contains(err.Error(), "editable fields") {
		t.Fatalf("unimplemented edit form was presented as read-only: %v", err)
	}
}

func TestSelfRefreshAccountUsesItsOwnTokenAndEmptyResponse(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Self/dashboard" {
			fmt.Fprint(w, selfTestPage("", "fixture-account"))
			return
		}
		if r.URL.Path != "/Self/dashboard/refreshaccount" || r.URL.Query().Get("csrftoken") != "refresh-token" || r.URL.Query().Has("ajaxCsrfToken") {
			t.Error("refreshaccount used an incorrect path or token")
		}
	})
	err := s.RefreshAccount(context.Background())
	if err != nil {
		t.Fatalf("expected current empty-body contract, got %v", err)
	}
}
