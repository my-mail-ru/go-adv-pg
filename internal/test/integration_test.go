//go:build integration

package advpgtest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	advpg "github.com/my-mail-ru/go-adv-pg"
)

func must(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func getTestDSN() string {
	if dsn := os.Getenv("ADVPG_TEST_DSN"); dsn != "" {
		return dsn
	}

	return "postgresql://testuser:testpasswd@localhost/testdb"
}

func connectDB(t *testing.T) (context.Context, advpg.DB) {
	ctx := t.Context()

	db, err := pgx.Connect(ctx, getTestDSN())
	if err != nil {
		t.Fatal(err)
	}

	return ctx, db
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
	ctx, db := connectDB(t)
	userDAO := NewUserDAO(db)
	userID := 0

	t.Run("InitByStorage", func(t *testing.T) {
		user := User{
			Name: "Test User",
			Type: 2,
		}.NewTuple()

		must(t, userDAO.Insert(ctx, user))

		if user.GetID() == 0 {
			t.Fatal("ID isn't set")
		}

		userID = user.GetID()

		if user.GetCreatedAt().IsZero() {
			t.Fatal("CreatedAt isn't set")
		}

		if user.GetUpdatedAt().IsZero() {
			t.Fatal("UpdatedAt isn't set")
		}

		if !user.GetCreatedAt().Equal(user.GetUpdatedAt()) {
			t.Fatalf("CreatedAt(%v) != UpdatedAt(%v)", user.GetCreatedAt(), user.GetUpdatedAt())
		}
	})

	t.Run("DisableUpdate", func(t *testing.T) {
		_, failed := any(User{}.NewTuple()).(interface{ SetCreatedAt(time.Time) })
		if failed {
			t.Fatal("UserTuple shoultn't have SetCreatedAt method, but it has")
		}
	})

	t.Run("UpdateByStorage", func(t *testing.T) {
		user, err := userDAO.SelectByID(ctx, userID)
		must(t, err)

		user.SetName(user.GetName() + " Updated")
		must(t, userDAO.Update(ctx, &user))

		if !user.GetCreatedAt().Before(user.GetUpdatedAt()) {
			t.Fatalf("CreatedAt(%v) should be before UpdatedAt(%v)", user.GetCreatedAt(), user.GetUpdatedAt())
		}
	})

	t.Run("Mutators", func(t *testing.T) {
		user1, err := userDAO.SelectByID(ctx, userID)
		must(t, err)

		initialPostCount := user1.GetPostCount()
		if initialPostCount != 1 {
			t.Fatalf("EnableMutators with InitByStorage failed: got PostCount=%d, but 1 was expected", initialPostCount)
		}

		user2, err := userDAO.SelectByID(ctx, userID)
		must(t, err)

		user1.IncPostCount()
		user2.IncPostCount()

		must(t, userDAO.Update(ctx, &user1))
		must(t, userDAO.Update(ctx, &user2))

		if user2.GetPostCount() != initialPostCount+2 {
			t.Fatalf("parallel mutator update: PostCount=%d, but %d was expected", user2.GetPostCount(), initialPostCount+2)
		}

		user1.IncPostCount()

		must(t, userDAO.Update(ctx, &user1))
		must(t, userDAO.Update(ctx, &user2)) // Update of unchanged record should be converted to querySelectMutators

		if user2.GetPostCount() != initialPostCount+3 {
			t.Fatalf("unchanged update: PostCount=%d, but %d was expected", user2.GetPostCount(), initialPostCount+3)
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
		}.NewTuple()

		must(t, userDAO.Insert(ctx, user2))

		got, err := userDAO.SelectMultiByIDType(ctx, []SelectMultiByIDTypeKey{
			{ID: userID, Type: user1.GetType()},
			{ID: -10, Type: -30},
			{ID: user2.GetID(), Type: user2.GetType()},
		})

		must(t, err)

		want := []UserTuple{*user2, user1}

		if diff := cmp.Diff(want, got, cmpopts.EquateComparable(UserTuple{})); diff != "" {
			t.Fatal("result mismatch (-want +got):\n" + diff)
		}
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
		}.NewTuple()

		err := userDAO.FullUpdate(ctx, user)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("got %v, expected sql.ErrNoRows", err)
		}
	})

	t.Run("Update non-existent key", func(t *testing.T) {
		user := User{
			ID: -10,
		}.NewTuple()

		user.SetName("foobar")

		err := userDAO.Update(ctx, user)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("got %v, expected sql.ErrNoRows", err)
		}
	})
}

func createUserID(t *testing.T, db advpg.DB) int {
	var userID int

	t.Run("create UserID", func(t *testing.T) {
		user := User{
			Name: "Test User",
			Type: 2,
		}.NewTuple()

		userDAO := NewUserDAO(db)

		must(t, userDAO.Insert(t.Context(), user))

		if user.GetID() == 0 {
			t.Fatal("ID isn't set")
		}

		userID = user.GetID()
	})

	return userID
}

func TestExtLinkDAO(t *testing.T) {
	ctx, db := connectDB(t)
	extDAO := NewExtLinkDAO(db)
	now := MyTime{Time: time.Now().Round(time.Second)}
	userID := createUserID(t, db)

	t.Run("UpdateOnConflict", func(t *testing.T) {
		ext := ExtLink{
			UserID:     userID,
			ExternalID: 123,
			CreatedAt:  now,
			Status:     1,
		}.NewTuple()

		must(t, extDAO.Insert(ctx, ext))
		ext.SetStatus(2)
		must(t, extDAO.Insert(ctx, ext))

		got, err := extDAO.SelectByPrimaryKey(ctx, ext.GetUserID(), ext.GetExternalID())
		must(t, err)

		if got.GetStatus() != ext.GetStatus() {
			t.Fatalf("Status after Insert with UpdateOnConflict: got %d, want %d", got.GetStatus(), ext.GetStatus())
		}

		if got.GetCreatedAt() != now {
			t.Fatalf("CreatedAt: got %v, expected %v", got.GetCreatedAt(), now)
		}
	})

	t.Run("Mutators with UpdateOnConflict", func(t *testing.T) {
		ext := ExtLink{
			UserID:     userID,
			ExternalID: 1234,
			CreatedAt:  now,
			Status:     1,
		}.NewTuple()

		must(t, extDAO.Insert(ctx, ext))

		initialLinkCount := ext.GetLinkCount()
		if initialLinkCount != 1 {
			t.Fatalf("EnableMutators with InitByStorage failed: got LinkCount=%d, but 1 was expected", initialLinkCount)
		}

		ext.IncLinkCount()
		must(t, extDAO.Insert(ctx, ext))

		got, err := extDAO.SelectByPrimaryKey(ctx, ext.GetUserID(), ext.GetExternalID())
		must(t, err)

		if got.GetLinkCount() != initialLinkCount+1 {
			t.Fatalf("LinkCount after Insert with UpdateOnConflict: got %d, want %d", got.GetLinkCount(), initialLinkCount+1)
		}
	})
}

func TestUserViewsDAO(t *testing.T) {
	ctx, db := connectDB(t)
	viewsDAO := NewUserViewsDAO(db)
	userID := createUserID(t, db)

	initialViews := 3

	t.Run("Mutators without InitByStorage", func(t *testing.T) {
		views := UserViews{
			UserID: userID,
			Views:  initialViews,
		}.NewTuple()

		must(t, viewsDAO.Insert(ctx, views))

		views2, err := viewsDAO.SelectByUserID(ctx, userID)

		must(t, err)

		views2.IncViews()
		must(t, viewsDAO.Update(ctx, &views2))
		views.IncViews()
		must(t, viewsDAO.Insert(ctx, views))

		got, err := viewsDAO.SelectByUserID(ctx, userID)

		must(t, err)

		if got.GetViews() != initialViews+2 {
			t.Fatalf("Views after Insert with UpdateOnConflict: got %d, want %d", got.GetViews(), initialViews+2)
		}
	})
}

func TestSeenDAO(t *testing.T) {
	ctx, db := connectDB(t)
	seenDAO := NewSeenDAO(db)
	userID := createUserID(t, db)

	t.Run("OnConflictDoNothing", func(t *testing.T) {
		seen := Seen{
			UserID: userID,
		}.NewTuple()

		must(t, seenDAO.Insert(ctx, seen))

		origSeenAt := seen.GetSeenAt()

		seen.SetSeenAt(origSeenAt.Add(time.Hour))
		must(t, seenDAO.Insert(ctx, seen))

		got, err := seenDAO.SelectByUserID(ctx, userID)

		must(t, err)

		if got.GetSeenAt() != origSeenAt {
			t.Fatalf("SeenAt was modified, but it shouldn't: got %v, want %v", got.GetSeenAt(), origSeenAt)
		}
	})

	t.Run("Update unchanged record", func(t *testing.T) {
		seen, err := seenDAO.SelectByUserID(ctx, userID)
		must(t, err)

		seen.SetSeenAt(time.Now())
		must(t, seenDAO.Update(ctx, &seen))
		must(t, NewSeenDAO(errDB{}).Update(ctx, &seen))
	})
}
