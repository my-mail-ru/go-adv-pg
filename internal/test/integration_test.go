//go:build integration

package advpgtest

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/onlineconf/onlineconf-go/v2"

	advmetricsset "github.com/my-mail-ru/go-adv-metrics/set"
	advpg "github.com/my-mail-ru/go-adv-pg"
	advpgconn "github.com/my-mail-ru/go-adv-pg/conn"
)

const insertMultiCount = 100

func must(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func getConf(t *testing.T) advpgconn.OnlineConf {
	cdbName := os.Getenv("ADVPG_ONLINECONF")
	if cdbName == "" {
		cdbName = "testdata/config.cdb"
	}

	conf, err := onlineconf.OpenModule(cdbName)
	if err != nil {
		t.Fatal(err)
	}

	return conf
}

func connectDB(t *testing.T) (context.Context, advpg.DB, advmetricsset.Set) {
	ctx := t.Context()
	ms := advmetricsset.New()

	db, err := advpgconn.NewConn(ctx, getConf(t), advpgconn.WithConnMetrics(ms))
	if err != nil {
		t.Fatal(err)
	}

	return ctx, db, ms
}

type (
	errDB      struct{}
	errScanner struct {
		err error
	}
)

func (errDB) Query(_ context.Context, q string, args ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("unexpected Query: %q %v", q, args)
}

func (errDB) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	return errScanner{err: fmt.Errorf("unexpected QueryRow: %q %v", q, args)}
}

func (errDB) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, fmt.Errorf("unexpected Exec: %q %v", q, args)
}

func (es errScanner) Scan(...any) error {
	return es.err
}

func TestUserDAO(t *testing.T) {
	ctx, db, ms := connectDB(t)
	userDAO := NewUserDAO(db)
	userID := 0

	t.Run("InitByStorage", func(t *testing.T) {
		user := User{
			Name: "Test User",
			Type: 2,
		}.Record()

		must(t, userDAO.Insert(ctx, user))

		if user.ID() == 0 {
			t.Fatal("ID isn't set")
		}

		userID = user.ID()

		if user.CreatedAt().IsZero() {
			t.Fatal("CreatedAt isn't set")
		}

		if user.UpdatedAt().IsZero() {
			t.Fatal("UpdatedAt isn't set")
		}

		if !user.CreatedAt().Equal(user.UpdatedAt()) {
			t.Fatalf("CreatedAt(%v) != UpdatedAt(%v)", user.CreatedAt(), user.UpdatedAt())
		}
	})

	t.Run("DisableUpdate", func(t *testing.T) {
		_, failed := any(User{}.Record()).(interface{ SetCreatedAt(time.Time) })
		if failed {
			t.Fatal("UserRecord shouldn't have SetCreatedAt method, but it has")
		}
	})

	t.Run("UpdateByStorage", func(t *testing.T) {
		user, err := userDAO.SelectByID(ctx, userID, advpg.WithReplica(true))
		must(t, err)

		user.SetName(user.Name() + " Updated")
		must(t, userDAO.Update(ctx, &user))

		if !user.CreatedAt().Before(user.UpdatedAt()) {
			t.Fatalf("CreatedAt(%v) should be before UpdatedAt(%v)", user.CreatedAt(), user.UpdatedAt())
		}
	})

	t.Run("Mutators", func(t *testing.T) {
		user1, err := userDAO.SelectByID(ctx, userID)
		must(t, err)

		initialPostCount := user1.PostCount()
		if initialPostCount != 1 {
			t.Fatalf("EnableMutators with InitByStorage failed: got PostCount=%d, but 1 was expected", initialPostCount)
		}

		user2, err := userDAO.SelectByID(ctx, userID)
		must(t, err)

		user1.IncPostCount()
		user2.IncPostCount()

		must(t, userDAO.Update(ctx, &user1))
		must(t, userDAO.Update(ctx, &user2))

		if user2.PostCount() != initialPostCount+2 {
			t.Fatalf("parallel mutator update: PostCount=%d, but %d was expected", user2.PostCount(), initialPostCount+2)
		}

		user1.IncPostCount()

		must(t, userDAO.Update(ctx, &user1))
		must(t, userDAO.Update(ctx, &user2)) // Update of unchanged record should be converted to querySelectMutators

		if user2.PostCount() != initialPostCount+3 {
			t.Fatalf("unchanged update: PostCount=%d, but %d was expected", user2.PostCount(), initialPostCount+3)
		}
	})

	t.Run("UniqKeyNotFound", func(t *testing.T) {
		_, err := userDAO.SelectByID(ctx, -10)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("select by non-existent primary key returned %v, but sql.ErrNoRows was expected", err)
		}
	})

	t.Run("NonUniqKeyNotFound", func(t *testing.T) {
		results, err := userDAO.SelectByName(ctx, "NonExistentUser")
		must(t, err)

		if len(results) != 0 {
			t.Fatalf("select by non-existent non-uniq key returned %v, but an empty slice was expected", err)
		}
	})

	t.Run("MultiKeyNotFound", func(t *testing.T) {
		results, err := userDAO.SelectMultiByIDType(ctx, []SelectMultiByIDTypeKey{
			{ID: -10, Type: 1},
			{ID: -20, Type: 2},
		})

		must(t, err)

		if len(results) != 0 {
			t.Fatalf("select by non-existent multi key returned %v, but an empty slice was expected", err)
		}
	})

	t.Run("OrderBy", func(t *testing.T) {
		user1, err := userDAO.SelectByID(ctx, userID)
		must(t, err)

		user2 := User{
			Name: "Test User 2",
			Type: 3,
		}.Record()

		must(t, userDAO.Insert(ctx, user2))

		got, err := userDAO.SelectMultiByIDType(ctx, []SelectMultiByIDTypeKey{
			{ID: userID, Type: user1.Type()},
			{ID: -10, Type: -30},
			{ID: user2.ID(), Type: user2.Type()},
		})

		must(t, err)

		want := []UserRecord{*user2, user1}

		cmpSlices(t, got, want)
	})

	t.Run("Delete by non-existent key", func(t *testing.T) {
		err := userDAO.DeleteByID(ctx, -10)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("got %v, expected sql.ErrNoRows", err)
		}
	})

	t.Run("Delete by existing key", func(t *testing.T) {
		must(t, userDAO.DeleteByID(ctx, userID))
	})

	t.Run("FullUpdate non-existent key", func(t *testing.T) {
		user := User{
			ID:   -10,
			Name: "Foobar",
		}.Record()

		err := userDAO.FullUpdate(ctx, user)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("got %v, expected sql.ErrNoRows", err)
		}
	})

	t.Run("Update non-existent key", func(t *testing.T) {
		user := User{
			ID: -10,
		}.Record()

		user.SetName("foobar")

		err := userDAO.Update(ctx, user)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("got %v, expected sql.ErrNoRows", err)
		}
	})

	t.Run("InsertMulti", func(t *testing.T) {
		users := make([]UserRecord, insertMultiCount)
		for i := range insertMultiCount {
			users[i] = *(User{
				Name: "TestInsertMulti " + strconv.Itoa(i),
				Type: i,
			}.Record())
		}

		must(t, userDAO.InsertMulti(ctx, users))

		for i := range insertMultiCount - 1 {
			if users[i].ID() >= users[i+1].ID() {
				t.Fatalf("user IDs aren't monotonically increasing: users[%d].ID=%d, users[%d].ID=%d", i, users[i].ID(), i+1, users[i+1].ID())
			}
		}

		keys := make([]SelectMultiByIDTypeKey, len(users))
		for i, user := range users {
			keys[i] = SelectMultiByIDTypeKey{
				ID:   user.ID(),
				Type: user.Type(),
			}
		}

		gotUsers, err := userDAO.SelectMultiByIDType(ctx, keys)

		must(t, err)

		slices.SortFunc(gotUsers, func(x, y UserRecord) int {
			return cmp.Compare(x.ID(), y.ID())
		})

		cmpSlices(t, gotUsers, users)
	})

	checkMetrics(t, ms, expectedMetrics{
		{
			table:   "users",
			index:   "ID",
			command: "SELECT",
		}: 6,
		{
			table:   "users",
			index:   "Name",
			command: "SELECT",
		}: 1,
		{
			table:   "users",
			index:   "IDType",
			command: "SELECT",
		}: 3,
		{
			table:   "users",
			command: "INSERT",
		}: 3,
		{
			table:   "users",
			command: "UPDATE",
		}: 6,
		{
			table:   "users",
			index:   "ID",
			command: "DELETE",
		}: 2,
	})
}

type expectedMetrics map[expectedMetric]int

type expectedMetric struct {
	table   string
	index   string
	command string
}

var (
	metricRegexp = regexp.MustCompile(`(?m:^pgx_queries_total{([^}]+)} (\d+)$)`)
	labelsRegexp = regexp.MustCompile(`(?m:(?:^|,)(\w+)="(\w+)")`)
)

// TODO make adv-metrics/testing package with generalized checkers like this
func checkMetrics(t *testing.T, ms advmetricsset.Set, want expectedMetrics) {
	t.Run("check metrics", func(t *testing.T) {
		buf := &strings.Builder{}

		ms.WritePrometheus(buf)

		metricMatches := metricRegexp.FindAllStringSubmatch(buf.String(), -1)
		if len(metricMatches) == 0 {
			t.Fatal("no metrics found")
		}

		for _, metricMatch := range metricMatches {
			gotCount, err := strconv.Atoi(metricMatch[2])
			if err != nil {
				t.Fatal(metricMatch[0], ": invalid count:", err)
			}

			labelMatches := labelsRegexp.FindAllStringSubmatch(metricMatch[1], -1)
			if len(labelMatches) == 0 {
				t.Fatal("no labels:", metricMatch[0])
			}

			labels := make(map[string]string, len(labelMatches))
			for _, labelMatch := range labelMatches {
				if labelMatch[2] == "" {
					t.Fatal(labelMatch[1], "label is empty but it shouldn't")
				}

				labels[labelMatch[1]] = labelMatch[2]
			}

			got := expectedMetric{
				table:   labels["table"],
				index:   labels["index"],
				command: labels["command"],
			}

			wantCount, ok := want[got]
			if !ok {
				t.Fatalf("unexpected metric %#v", got)
			}

			if gotCount != wantCount {
				t.Fatalf("metric %#v: got count %d, want count %d", got, gotCount, wantCount)
			}

			delete(want, got)
		}

		for _, notFound := range want {
			t.Fatal("missing metric", notFound)
		}
	})
}

func createUserID(t *testing.T, db advpg.DB) int {
	var userID int

	t.Run("create UserID", func(t *testing.T) {
		user := User{
			Name: "Test User",
			Type: 2,
		}.Record()

		userDAO := NewUserDAO(db)

		must(t, userDAO.Insert(t.Context(), user))

		if user.ID() == 0 {
			t.Fatal("ID isn't set")
		}

		userID = user.ID()
	})

	return userID
}

func cmpSlices[T any](t *testing.T, got, want []T) {
	var val T

	if diff := gocmp.Diff(want, got, gocmpopts.EquateComparable(val)); diff != "" {
		t.Fatal("result mismatch (-want +got):\n" + diff)
	}
}

func TestExtLinkDAO(t *testing.T) {
	ctx, db, ms := connectDB(t)
	extDAO := NewExtLinkDAO(db)
	now := MyTime{Time: time.Now().Round(time.Second)}
	userID := createUserID(t, db)

	t.Run("UpdateOnConflict", func(t *testing.T) {
		ext := ExtLink{
			UserID:     userID,
			ExternalID: 123,
			CreatedAt:  now,
			Status:     1,
		}.Record()

		must(t, extDAO.Insert(ctx, ext))
		ext.SetStatus(2)
		must(t, extDAO.Insert(ctx, ext))

		got, err := extDAO.SelectByPrimaryKey(ctx, ext.UserID(), ext.ExternalID())
		must(t, err)

		if got.Status() != ext.Status() {
			t.Fatalf("Status after Insert with UpdateOnConflict: got %d, want %d", got.Status(), ext.Status())
		}

		if got.CreatedAt() != now {
			t.Fatalf("CreatedAt: got %v, expected %v", got.CreatedAt(), now)
		}
	})

	t.Run("Mutators with UpdateOnConflict", func(t *testing.T) {
		ext := ExtLink{
			UserID:     userID,
			ExternalID: 1234,
			CreatedAt:  now,
			Status:     1,
		}.Record()

		must(t, extDAO.Insert(ctx, ext))

		initialLinkCount := ext.LinkCount()
		if initialLinkCount != 1 {
			t.Fatalf("EnableMutators with InitByStorage failed: got LinkCount=%d, but 1 was expected", initialLinkCount)
		}

		ext.IncLinkCount()
		must(t, extDAO.Insert(ctx, ext))

		got, err := extDAO.SelectByPrimaryKey(ctx, ext.UserID(), ext.ExternalID())
		must(t, err)

		if got.LinkCount() != initialLinkCount+1 {
			t.Fatalf("LinkCount after Insert with UpdateOnConflict: got %d, want %d", got.LinkCount(), initialLinkCount+1)
		}
	})

	checkMetrics(t, ms, expectedMetrics{
		{
			table:   "users",
			command: "INSERT",
		}: 1,
		{
			table:   "ext_links",
			command: "INSERT",
		}: 4,
		{
			table:   "ext_links",
			index:   "PrimaryKey",
			command: "SELECT",
		}: 2,
	})
}

func TestUserViewsDAO(t *testing.T) {
	ctx, db, ms := connectDB(t)
	viewsDAO := NewUserViewsDAO(db)
	userID := createUserID(t, db)

	initialViews := 3

	t.Run("Mutators without InitByStorage", func(t *testing.T) {
		views := UserViews{
			UserID: userID,
			Views:  initialViews,
		}.Record()

		must(t, viewsDAO.Insert(ctx, views))

		views2, err := viewsDAO.SelectByUserID(ctx, userID)

		must(t, err)

		views2.IncViews()
		must(t, viewsDAO.Update(ctx, &views2))
		views.IncViews()
		must(t, viewsDAO.Insert(ctx, views))

		got, err := viewsDAO.SelectByUserID(ctx, userID)

		must(t, err)

		if got.Views() != initialViews+2 {
			t.Fatalf("Views after Insert with UpdateOnConflict: got %d, want %d", got.Views(), initialViews+2)
		}
	})

	checkMetrics(t, ms, expectedMetrics{
		{
			table:   "users",
			command: "INSERT",
		}: 1,
		{
			table:   "user_views",
			command: "INSERT",
		}: 2,
		{
			table:   "user_views",
			command: "UPDATE",
		}: 1,
		{
			table:   "user_views",
			index:   "UserID",
			command: "SELECT",
		}: 2,
	})
}

func TestSeenDAO(t *testing.T) {
	ctx, db, ms := connectDB(t)
	seenDAO := NewSeenDAO(db)
	userID := createUserID(t, db)

	t.Run("OnConflictDoNothing", func(t *testing.T) {
		seen := Seen{
			UserID: userID,
		}.Record()

		must(t, seenDAO.Insert(ctx, seen))

		origSeenAt := seen.SeenAt()

		seen.SetSeenAt(origSeenAt.Add(time.Hour))
		must(t, seenDAO.Insert(ctx, seen))

		got, err := seenDAO.SelectByUserID(ctx, userID)

		must(t, err)

		if got.SeenAt() != origSeenAt {
			t.Fatalf("SeenAt was modified, but it shouldn't: got %v, want %v", got.SeenAt(), origSeenAt)
		}
	})

	t.Run("Update unchanged record", func(t *testing.T) {
		seen, err := seenDAO.SelectByUserID(ctx, userID)
		must(t, err)

		seen.SetSeenAt(time.Now())
		must(t, seenDAO.Update(ctx, &seen))
		must(t, NewSeenDAO(errDB{}).Update(ctx, &seen))
	})

	checkMetrics(t, ms, expectedMetrics{
		{
			table:   "users",
			command: "INSERT",
		}: 1,
		{
			table:   "seen",
			command: "INSERT",
		}: 2,
		{
			table:   "seen",
			index:   "UserID",
			command: "SELECT",
		}: 2,
		{
			table:   "seen",
			command: "UPDATE",
		}: 1,
	})
}

func TestUpdateMultiUserDAO(t *testing.T) {
	ctx, db, _ := connectDB(t)
	userDAO := NewUserDAO(db)

	// Insert 3 users
	users := make([]UserRecord, 3)
	for i := range users {
		users[i] = *(User{
			Name: "UpdateMulti User " + strconv.Itoa(i),
			Type: i + 1,
		}.Record())
	}
	must(t, userDAO.InsertMulti(ctx, users))

	t.Run("multiple columns and UpdateByStorage", func(t *testing.T) {
		// Modify multiple columns
		users[0].SetName("Changed 0")
		users[0].SetType(10)
		users[1].SetName("Changed 1")
		users[1].SetType(20)
		users[2].SetName("Changed 2")
		users[2].SetType(30)

		updatedAtBefore := make([]time.Time, len(users))
		for i := range users {
			updatedAtBefore[i] = users[i].UpdatedAt()
		}

		must(t, userDAO.UpdateMulti(ctx, users))

		// Verify changes persisted
		for i, u := range users {
			got, err := userDAO.SelectByID(ctx, u.ID())
			must(t, err)

			wantName := "Changed " + strconv.Itoa(i)
			if got.Name() != wantName {
				t.Fatalf("users[%d].Name: got %q, want %q", i, got.Name(), wantName)
			}

			wantType := (i + 1) * 10
			if got.Type() != wantType {
				t.Fatalf("users[%d].Type: got %d, want %d", i, got.Type(), wantType)
			}

			// UpdateByStorage: updated_at must be refreshed by the BEFORE UPDATE trigger
			if !updatedAtBefore[i].Before(users[i].UpdatedAt()) {
				t.Fatalf("users[%d].UpdatedAt wasn't refreshed by trigger: before=%v, after=%v", i, updatedAtBefore[i], users[i].UpdatedAt())
			}
		}
	})

	t.Run("mutators", func(t *testing.T) {
		initialPostCounts := make([]int, len(users))
		for i := range users {
			initialPostCounts[i] = users[i].PostCount()
		}

		users[0].IncPostCount()
		users[1].AddPostCount(5)
		// users[2] gets 0 mutator delta — still valid

		must(t, userDAO.UpdateMulti(ctx, users))

		for i, u := range users {
			got, err := userDAO.SelectByID(ctx, u.ID())
			must(t, err)

			var wantDelta int
			switch i {
			case 0:
				wantDelta = 1
			case 1:
				wantDelta = 5
			}

			if got.PostCount() != initialPostCounts[i]+wantDelta {
				t.Fatalf("users[%d].PostCount: got %d, want %d", i, got.PostCount(), initialPostCounts[i]+wantDelta)
			}
		}
	})

	t.Run("missing keys with RETURNING", func(t *testing.T) {
		records := []UserRecord{
			users[0],
			*(User{ID: -999, Name: "ghost", Type: 0}.Record()),
		}

		err := userDAO.UpdateMulti(ctx, records)
		if err == nil {
			t.Fatal("expected error for partial RETURNING result, got nil")
		}
		if !strings.Contains(err.Error(), "got") || !strings.Contains(err.Error(), "expected") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})
}

func TestUpdateMultiExtLinkDAO(t *testing.T) {
	ctx, db, _ := connectDB(t)
	extDAO := NewExtLinkDAO(db)
	userID := createUserID(t, db)
	now := MyTime{Time: time.Now().Round(time.Second)}

	// Insert 2 ext_links
	exts := make([]ExtLinkRecord, 2)
	for i := range exts {
		exts[i] = *(ExtLink{
			UserID:     userID,
			ExternalID: 100 + i,
			CreatedAt:  now,
			Status:     i,
		}.Record())
	}

	for i := range exts {
		must(t, extDAO.Insert(ctx, &exts[i]))
	}

	t.Run("mutators with RETURNING", func(t *testing.T) {
		initialCounts := make([]int, len(exts))
		for i := range exts {
			initialCounts[i] = exts[i].LinkCount()
		}

		exts[0].IncLinkCount()
		exts[1].AddLinkCount(3)

		must(t, extDAO.UpdateMulti(ctx, exts))

		for i := range exts {
			got, err := extDAO.SelectByPrimaryKey(ctx, userID, 100+i)
			must(t, err)

			var wantDelta int
			switch i {
			case 0:
				wantDelta = 1
			case 1:
				wantDelta = 3
			}

			wantCount := initialCounts[i] + wantDelta
			if got.LinkCount() != wantCount {
				t.Fatalf("exts[%d].LinkCount: got %d, want %d", i, got.LinkCount(), wantCount)
			}

			// RETURNING value should match
			if exts[i].LinkCount() != wantCount {
				t.Fatalf("exts[%d].LinkCount from RETURNING: got %d, want %d", i, exts[i].LinkCount(), wantCount)
			}
		}
	})
}

func TestUpdateMultiUserOptionsDAO(t *testing.T) {
	ctx, db, _ := connectDB(t)
	optDAO := NewUserOptionsDAO(db)
	userID := createUserID(t, db)

	// Insert 3 options
	opts := make([]UserOptionsRecord, 3)
	for i := range opts {
		opts[i] = *(UserOptions{
			UserID:   userID,
			OptionID: i,
			Flag:     false,
			Option:   "original " + strconv.Itoa(i),
		}.Record())
	}
	must(t, optDAO.InsertMulti(ctx, opts))

	t.Run("multiple columns changed", func(t *testing.T) {
		opts[0].SetFlag(true)
		opts[0].SetOption("changed 0")
		opts[1].SetFlag(true)
		opts[1].SetOption("changed 1")
		opts[2].SetOption("changed 2")

		must(t, optDAO.UpdateMulti(ctx, opts))

		gotOpts, err := optDAO.SelectByUserID(ctx, userID)
		must(t, err)
		cmpSlices(t, gotOpts, opts)
	})

	t.Run("missing keys without RETURNING", func(t *testing.T) {
		records := []UserOptionsRecord{
			opts[0],
			*(UserOptions{UserID: -999, OptionID: -1, Flag: true, Option: "ghost"}.Record()),
		}

		// No RETURNING → Exec path → no error for missing keys
		err := optDAO.UpdateMulti(ctx, records)
		if err != nil {
			t.Fatalf("expected no error for missing keys without RETURNING, got: %v", err)
		}
	})
}

func TestUserOptionsDAO(t *testing.T) {
	ctx, db, ms := connectDB(t)
	optDAO := NewUserOptionsDAO(db)
	userID := createUserID(t, db)

	t.Run("InsertMulti", func(t *testing.T) {
		opts := make([]UserOptionsRecord, insertMultiCount)
		for i := range insertMultiCount {
			opts[i] = *(UserOptions{
				UserID:   userID,
				OptionID: i,
				Flag:     i%2 != 0,
			}.Record())
		}

		must(t, optDAO.InsertMulti(ctx, opts))

		opts[insertMultiCount/2].SetFlag(!opts[insertMultiCount/2].Flag())
		opts[insertMultiCount/4].SetOption("CHANGED")
		must(t, optDAO.InsertMulti(ctx, opts))

		gotOpts, err := optDAO.SelectByUserID(ctx, userID)

		must(t, err)
		cmpSlices(t, gotOpts, opts)
	})

	checkMetrics(t, ms, expectedMetrics{
		{
			table:   "users",
			command: "INSERT",
		}: 1,
		{
			table:   "user_options",
			command: "INSERT",
		}: 2,
		{
			table:   "user_options",
			index:   "UserID",
			command: "SELECT",
		}: 1,
	})
}

func TestDeleteUserDAO(t *testing.T) {
	ctx, db, _ := connectDB(t)
	userDAO := NewUserDAO(db)

	t.Run("Delete non-existent record", func(t *testing.T) {
		user := User{ID: -10}.Record()
		err := userDAO.Delete(ctx, user)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("got %v, expected sql.ErrNoRows", err)
		}
	})

	t.Run("Delete existing record", func(t *testing.T) {
		user := User{Name: "ToDelete", Type: 1}.Record()
		must(t, userDAO.Insert(ctx, user))

		must(t, userDAO.Delete(ctx, user))

		_, err := userDAO.SelectByID(ctx, user.ID())
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("record should be deleted, but SelectByID returned: %v", err)
		}
	})

	t.Run("DeleteMulti empty slice", func(t *testing.T) {
		must(t, userDAO.DeleteMulti(ctx, nil))
	})

	t.Run("DeleteMulti", func(t *testing.T) {
		users := make([]UserRecord, 3)
		for i := range users {
			users[i] = *(User{
				Name: "DeleteMulti " + strconv.Itoa(i),
				Type: i,
			}.Record())
		}
		must(t, userDAO.InsertMulti(ctx, users))

		// Mix existing and non-existing records; DeleteMulti must not error.
		records := make([]UserRecord, len(users), len(users)+1)
		copy(records, users)
		records = append(records, *(User{ID: -999}.Record()))
		must(t, userDAO.DeleteMulti(ctx, records))

		for i, u := range users {
			_, err := userDAO.SelectByID(ctx, u.ID())
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("users[%d] should be deleted, but SelectByID returned: %v", i, err)
			}
		}
	})
}

func TestDeleteExtLinkDAO(t *testing.T) {
	ctx, db, _ := connectDB(t)
	extDAO := NewExtLinkDAO(db)
	userID := createUserID(t, db)
	now := MyTime{Time: time.Now().Round(time.Second)}

	t.Run("Delete non-existent record", func(t *testing.T) {
		ext := ExtLink{UserID: -10, ExternalID: -20}.Record()
		err := extDAO.Delete(ctx, ext)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("got %v, expected sql.ErrNoRows", err)
		}
	})

	t.Run("Delete existing record", func(t *testing.T) {
		ext := ExtLink{UserID: userID, ExternalID: 500, CreatedAt: now, Status: 1}.Record()
		must(t, extDAO.Insert(ctx, ext))

		must(t, extDAO.Delete(ctx, ext))

		_, err := extDAO.SelectByPrimaryKey(ctx, userID, 500)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("record should be deleted, but SelectByPrimaryKey returned: %v", err)
		}
	})

	t.Run("DeleteMulti", func(t *testing.T) {
		exts := make([]ExtLinkRecord, 3)
		for i := range exts {
			exts[i] = *(ExtLink{
				UserID:     userID,
				ExternalID: 600 + i,
				CreatedAt:  now,
				Status:     i,
			}.Record())
		}
		for i := range exts {
			must(t, extDAO.Insert(ctx, &exts[i]))
		}

		must(t, extDAO.DeleteMulti(ctx, exts))

		for i, ext := range exts {
			_, err := extDAO.SelectByPrimaryKey(ctx, ext.UserID(), ext.ExternalID())
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("exts[%d] should be deleted, but SelectByPrimaryKey returned: %v", i, err)
			}
		}
	})
}
