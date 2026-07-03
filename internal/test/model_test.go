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
		if !strings.Contains(sql, "modified_at=(t::ext_links).modified_at") {
			t.Error("missing SET modified_at:", sql)
		}
		if !strings.Contains(sql, "link_count=ext_links.link_count+(t::ext_links).link_count") {
			t.Error("missing mutator SET:", sql)
		}
		if !strings.Contains(sql, "RETURNING ext_links.link_count") {
			t.Error("missing RETURNING link_count:", sql)
		}
		if !strings.Contains(sql, "EXTRACT(EPOCH FROM ext_links.refreshed_at::TIMESTAMP WITH TIME ZONE)::BIGINT AS refreshed_at") {
			t.Error("missing RETURNING refreshed_at with SQLScan:", sql)
		}
	})
}

func TestSQLValueInUpdate(t *testing.T) {
	t.Run("smart Update with SQLValue", func(t *testing.T) {
		m := ExtLink{UserID: 1, ExternalID: 2, Status: 3}.Record()
		m.SetModifiedAt(MyTime{})
		q := m.queryUpdate()
		sql := q.SQL()

		if !strings.Contains(sql, "modified_at=TIMESTAMP WITH TIME ZONE 'epoch' + INTERVAL '1 sec' * $") {
			t.Error("missing SQLValue in smart Update SET:", sql)
		}
		if !strings.Contains(sql, "RETURNING link_count") {
			t.Error("missing RETURNING link_count:", sql)
		}
		if !strings.Contains(sql, "EXTRACT(EPOCH FROM refreshed_at::TIMESTAMP WITH TIME ZONE)::BIGINT AS refreshed_at") {
			t.Error("missing RETURNING refreshed_at with SQLScan:", sql)
		}
	})

	t.Run("FullUpdate with SQLValue", func(t *testing.T) {
		m := ExtLink{UserID: 1, ExternalID: 2, Status: 3}.Record()
		q := m.queryFullUpdate()
		sql := q.SQL()

		if !strings.Contains(sql, "modified_at=TIMESTAMP WITH TIME ZONE 'epoch' + INTERVAL '1 sec' * $2") {
			t.Error("missing SQLValue in FullUpdate SET:", sql)
		}
		if !strings.Contains(sql, "RETURNING link_count") {
			t.Error("missing RETURNING link_count:", sql)
		}
		if !strings.Contains(sql, "EXTRACT(EPOCH FROM refreshed_at::TIMESTAMP WITH TIME ZONE)::BIGINT AS refreshed_at") {
			t.Error("missing RETURNING refreshed_at with SQLScan:", sql)
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("User single PK", func(t *testing.T) {
		m := User{ID: 42, Name: "test"}.Record()
		q := m.queryDelete()
		sql := q.SQL()

		if sql != `DELETE FROM users WHERE id=$1` {
			t.Error("unexpected SQL:", sql)
		}
		if len(q.Args()) != 1 {
			t.Errorf("expected 1 arg, got %d", len(q.Args()))
		}
	})

	t.Run("ExtLink composite PK", func(t *testing.T) {
		m := ExtLink{UserID: 1, ExternalID: 2}.Record()
		q := m.queryDelete()
		sql := q.SQL()

		if sql != `DELETE FROM ext_links WHERE user_id=$1 AND ext_id=$2` {
			t.Error("unexpected SQL:", sql)
		}
		if len(q.Args()) != 2 {
			t.Errorf("expected 2 args, got %d", len(q.Args()))
		}
	})
}

func checkIntArgs(t *testing.T, got []any, want ...int) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d args, got %d: %v", len(want), len(got), got)
	}

	for i, w := range want {
		if got[i] != w {
			t.Errorf("arg %d: got %v (%T), want %d", i, got[i], got[i], w)
		}
	}
}

// TestSelectCondition and TestDeleteCondition lock down the generated WHERE clause
// (and its args) across the 2x2 of {single vs multiple key columns} x {single vs
// multiple values}:
//
//	                | single value                | multiple values
//	----------------+-----------------------------+----------------------------------
//	single column   | name=$1                     | status=ANY($1)
//	multiple columns| user_id=$1 AND ext_id=$2    | (id, type) IN (($1, $2),($3, $4))
func TestSelectCondition(t *testing.T) {
	t.Run("1 column, 1 value", func(t *testing.T) {
		q := User{}.Record().querySelectByName("bob", 0, 0)

		if !strings.Contains(q.SQL(), "WHERE name=$1") {
			t.Error("unexpected condition:", q.SQL())
		}
		if got := q.Args(); len(got) != 1 || got[0] != "bob" {
			t.Errorf("unexpected args: %v", got)
		}
	})

	t.Run("1 column, N values (ANY)", func(t *testing.T) {
		q := ExtLink{}.Record().querySelectMultiByStatus([]int{1, 2, 3}, 0, 0)

		if !strings.Contains(q.SQL(), "WHERE status=ANY($1)") {
			t.Error("unexpected condition:", q.SQL())
		}
		if got := q.Args(); len(got) != 1 {
			t.Errorf("expected 1 arg (the slice), got %d: %v", len(got), got)
		}
	})

	t.Run("N columns, 1 value", func(t *testing.T) {
		q := ExtLink{}.Record().querySelectByPrimaryKey(10, 20, 0, 0)

		if !strings.Contains(q.SQL(), "WHERE user_id=$1 AND ext_id=$2") {
			t.Error("unexpected condition:", q.SQL())
		}
		checkIntArgs(t, q.Args(), 10, 20)
	})

	t.Run("N columns, N values (row-value IN)", func(t *testing.T) {
		q := User{}.Record().querySelectMultiByIDType([]SelectMultiByIDTypeKey{
			{ID: 1, Type: 2},
			{ID: 3, Type: 4},
		}, 0, 0)

		if !strings.Contains(q.SQL(), "WHERE (id, type) IN (($1, $2),($3, $4))") {
			t.Error("unexpected condition:", q.SQL())
		}
		checkIntArgs(t, q.Args(), 1, 2, 3, 4)
	})

	t.Run("N columns, N values, empty slice", func(t *testing.T) {
		q := User{}.Record().querySelectMultiByIDType(nil, 0, 0)

		if q.SQL() != "" {
			t.Error("expected empty SQL for empty key slice, got:", q.SQL())
		}
	})
}

func TestDeleteCondition(t *testing.T) {
	t.Run("1 column, 1 value", func(t *testing.T) {
		q := User{}.Record().queryDeleteByName("bob")

		if !strings.Contains(q.SQL(), "WHERE name=$1") {
			t.Error("unexpected condition:", q.SQL())
		}
		if got := q.Args(); len(got) != 1 || got[0] != "bob" {
			t.Errorf("unexpected args: %v", got)
		}
	})

	t.Run("1 column, N values (ANY)", func(t *testing.T) {
		q := ExtLink{}.Record().queryDeleteMultiByStatus([]int{1, 2, 3})

		if !strings.Contains(q.SQL(), "WHERE status=ANY($1)") {
			t.Error("unexpected condition:", q.SQL())
		}
		if got := q.Args(); len(got) != 1 {
			t.Errorf("expected 1 arg (the slice), got %d: %v", len(got), got)
		}
	})

	t.Run("N columns, 1 value", func(t *testing.T) {
		q := ExtLink{}.Record().queryDeleteByPrimaryKey(10, 20)

		if !strings.Contains(q.SQL(), "WHERE user_id=$1 AND ext_id=$2") {
			t.Error("unexpected condition:", q.SQL())
		}
		checkIntArgs(t, q.Args(), 10, 20)
	})

	t.Run("N columns, N values (row-value IN)", func(t *testing.T) {
		q := User{}.Record().queryDeleteMultiByIDType([]SelectMultiByIDTypeKey{
			{ID: 1, Type: 2},
			{ID: 3, Type: 4},
		})

		if !strings.Contains(q.SQL(), "WHERE (id, type) IN (($1, $2),($3, $4))") {
			t.Error("unexpected condition:", q.SQL())
		}
		checkIntArgs(t, q.Args(), 1, 2, 3, 4)
	})

	t.Run("N columns, N values, empty slice", func(t *testing.T) {
		q := User{}.Record().queryDeleteMultiByIDType(nil)

		if q.SQL() != "" {
			t.Error("expected empty SQL for empty key slice, got:", q.SQL())
		}
	})
}

func TestDeleteMulti(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		q := queryDeleteMultiUser(nil)
		if q.SQL() != "" {
			t.Error("expected empty SQL for nil slice, got:", q.SQL())
		}
	})

	t.Run("User single PK", func(t *testing.T) {
		m1 := User{ID: 1}.Record()
		m2 := User{ID: 2}.Record()
		q := queryDeleteMultiUser([]UserRecord{*m1, *m2})
		sql := q.SQL()

		if !strings.Contains(sql, "DELETE FROM users WHERE") {
			t.Error("missing DELETE FROM:", sql)
		}
		if !strings.Contains(sql, "id IN (") {
			t.Error("missing IN clause:", sql)
		}
		if len(q.Args()) != 2 {
			t.Errorf("expected 2 args, got %d", len(q.Args()))
		}
	})

	t.Run("ExtLink composite PK", func(t *testing.T) {
		m1 := ExtLink{UserID: 1, ExternalID: 10}.Record()
		m2 := ExtLink{UserID: 2, ExternalID: 20}.Record()
		q := queryDeleteMultiExtLink([]ExtLinkRecord{*m1, *m2})
		sql := q.SQL()

		if !strings.Contains(sql, "DELETE FROM ext_links WHERE") {
			t.Error("missing DELETE FROM:", sql)
		}
		if !strings.Contains(sql, "(user_id, ext_id) IN (") {
			t.Error("missing composite IN clause:", sql)
		}
		if len(q.Args()) != 4 {
			t.Errorf("expected 4 args (2 per record), got %d", len(q.Args()))
		}
	})

	t.Run("UserOptions composite PK", func(t *testing.T) {
		m1 := UserOptions{UserID: 1, OptionID: 10}.Record()
		q := queryDeleteMultiUserOptions([]UserOptionsRecord{*m1})
		sql := q.SQL()

		if !strings.Contains(sql, "(user_id, option_id) IN (") {
			t.Error("missing composite IN clause:", sql)
		}
		if len(q.Args()) != 2 {
			t.Errorf("expected 2 args, got %d", len(q.Args()))
		}
	})
}

func TestSelectLimitOffset(t *testing.T) {
	t.Run("DefaultLimit applied", func(t *testing.T) {
		m := UserOptions{}.Record()
		q := m.querySelectByUserID(1, 50, 0)
		sql := q.SQL()

		if !strings.Contains(sql, "LIMIT 50") {
			t.Error("missing LIMIT 50:", sql)
		}
		if strings.Contains(sql, "OFFSET") {
			t.Error("unexpected OFFSET:", sql)
		}
	})

	t.Run("WithLimit overrides DefaultLimit", func(t *testing.T) {
		m := UserOptions{}.Record()
		q := m.querySelectByUserID(1, 10, 0)
		sql := q.SQL()

		if !strings.Contains(sql, "LIMIT 10") {
			t.Error("missing LIMIT 10:", sql)
		}
		if strings.Contains(sql, "LIMIT 50") {
			t.Error("should not contain default LIMIT 50:", sql)
		}
	})

	t.Run("WithOffset applied", func(t *testing.T) {
		m := UserOptions{}.Record()
		q := m.querySelectByUserID(1, 10, 5)
		sql := q.SQL()

		if !strings.Contains(sql, "LIMIT 10") {
			t.Error("missing LIMIT 10:", sql)
		}
		if !strings.Contains(sql, "OFFSET 5") {
			t.Error("missing OFFSET 5:", sql)
		}
	})

	t.Run("no limit or offset", func(t *testing.T) {
		m := User{}.Record()
		q := m.querySelectByName("test", 0, 0)
		sql := q.SQL()

		if strings.Contains(sql, "LIMIT") {
			t.Error("unexpected LIMIT:", sql)
		}
		if strings.Contains(sql, "OFFSET") {
			t.Error("unexpected OFFSET:", sql)
		}
	})

	t.Run("multi-keyset with limit and offset", func(t *testing.T) {
		m := User{}.Record()
		q := m.querySelectMultiByIDType([]SelectMultiByIDTypeKey{
			{ID: 1, Type: 1},
		}, 20, 3)
		sql := q.SQL()

		if !strings.Contains(sql, "LIMIT 20") {
			t.Error("missing LIMIT 20:", sql)
		}
		if !strings.Contains(sql, "OFFSET 3") {
			t.Error("missing OFFSET 3:", sql)
		}
	})

	t.Run("offset only", func(t *testing.T) {
		m := User{}.Record()
		q := m.querySelectByName("test", 0, 10)
		sql := q.SQL()

		if strings.Contains(sql, "LIMIT") {
			t.Error("unexpected LIMIT:", sql)
		}
		if !strings.Contains(sql, "OFFSET 10") {
			t.Error("missing OFFSET 10:", sql)
		}
	})
}
