//go:build !integration

package advpgtest

import (
	"strings"
	"testing"
)

func TestUserUpdate(t *testing.T) {
	m := User{}.Record()
	m.SetName("test")

	q := m.queryUpdate()
	sql := q.SQL()

	if !strings.Contains(sql, "name=$") {
		t.Error("missing field name:", q)
	}

	if strings.Contains(sql, "type=$") {
		t.Error("extra field type:", q)
	}
}

func TestUpdateMulti(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		q := queryUpdateMultiUserOptions(nil)
		if q.SQL() != "" {
			t.Error("expected empty SQL for nil slice, got:", q.SQL())
		}
	})

	t.Run("UserOptions no returning", func(t *testing.T) {
		m1 := UserOptions{UserID: 1, OptionID: 10, Flag: true, Option: "a"}.Record()
		m2 := UserOptions{UserID: 2, OptionID: 20, Flag: false, Option: "b"}.Record()
		q := queryUpdateMultiUserOptions([]UserOptionsRecord{*m1, *m2})
		sql := q.SQL()

		if !strings.Contains(sql, "UPDATE user_options SET") {
			t.Error("missing UPDATE head:", sql)
		}
		if !strings.Contains(sql, "flag=(t::user_options).flag") {
			t.Error("missing SET flag:", sql)
		}
		if !strings.Contains(sql, "option=(t::user_options).option") {
			t.Error("missing SET option:", sql)
		}
		if !strings.Contains(sql, ") t WHERE") {
			t.Error("missing ) t WHERE:", sql)
		}
		if !strings.Contains(sql, "user_options.user_id=(t::user_options).user_id") {
			t.Error("missing WHERE pk match:", sql)
		}
		if strings.Contains(sql, "RETURNING") {
			t.Error("unexpected RETURNING:", sql)
		}
		if len(q.Args()) != 8 {
			t.Errorf("expected 8 args (4 per record), got %d", len(q.Args()))
		}
	})

	t.Run("User with returning and mutator", func(t *testing.T) {
		m1 := User{Name: "alice", Type: 1}.Record()
		m1.IncPostCount()
		m2 := User{Name: "bob", Type: 2}.Record()
		m2.AddPostCount(5)
		q := queryUpdateMultiUser([]UserRecord{*m1, *m2})
		sql := q.SQL()

		if !strings.Contains(sql, "UPDATE users SET") {
			t.Error("missing UPDATE head:", sql)
		}
		if !strings.Contains(sql, "name=(t::users).name") {
			t.Error("missing SET name:", sql)
		}
		if !strings.Contains(sql, "post_count=users.post_count+(t::users).post_count") {
			t.Error("missing mutator SET:", sql)
		}
		if !strings.Contains(sql, "RETURNING users.post_count") {
			t.Error("missing RETURNING:", sql)
		}
		if len(q.Args()) != 8 {
			t.Errorf("expected 8 args (4 per record), got %d", len(q.Args()))
		}
		if len(q.Results()) != 4 {
			t.Errorf("expected 4 results (2 per record), got %d", len(q.Results()))
		}
	})

	t.Run("ExtLink with SQLValue in SET", func(t *testing.T) {
		q := queryUpdateMultiExtLink([]ExtLinkRecord{*ExtLink{UserID: 1, ExternalID: 2, Status: 3}.Record()})
		sql := q.SQL()

		if !strings.Contains(sql, "status=(t::ext_links).status") {
			t.Error("missing SET status:", sql)
		}
		if !strings.Contains(sql, "link_count=ext_links.link_count+(t::ext_links).link_count") {
			t.Error("missing mutator SET:", sql)
		}
		if !strings.Contains(sql, "RETURNING ext_links.link_count") {
			t.Error("missing RETURNING:", sql)
		}
	})
}
