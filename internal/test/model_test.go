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
