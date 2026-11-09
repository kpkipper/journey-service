package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kpkipper/journey-service/internal/admin"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/internal/storage"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportCLI(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	for _, n := range []string{"alice", "bob"} {
		require.NoError(t, db.Create(&models.User{ID: n, Email: n + "@x.com", Username: n}).Error)
	}
	rs := services.NewReportService(services.ReportDeps{
		Reports: repository.NewReportRepository(db), Users: repository.NewUserRepository(db)})
	r1, _, err := rs.Create(ctx, "bob", services.ReportInput{TargetType: "user", Target: "alice", Reason: "spam", Note: "line1\nline2"})
	require.NoError(t, err)
	r2, _, err := rs.Create(ctx, "alice", services.ReportInput{TargetType: "user", Target: "bob", Reason: "other"})
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, admin.PrintOpenReports(ctx, &buf, rs))
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2)
	var got models.Report
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &got))
	assert.Equal(t, r1.ID, got.ID)
	assert.Equal(t, "alice", got.TargetID)
	assert.Equal(t, "line1\nline2", got.Note)

	require.NoError(t, admin.ResolveReport(ctx, rs, r1.ID))
	buf.Reset()
	require.NoError(t, admin.PrintOpenReports(ctx, &buf, rs))
	assert.Contains(t, buf.String(), r2.ID)
	assert.NotContains(t, buf.String(), r1.ID)

	assert.ErrorIs(t, admin.ResolveReport(ctx, rs, "missing"), services.ErrReportNotFound)
}

func TestDeleteMomentCLI(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	store := storage.NewMemoryStore()
	require.NoError(t, db.Create(&models.User{ID: "alice", Email: "a@x.com", Username: "alice"}).Error)
	j := models.Journey{OwnerID: "alice", Title: "t", Destination: "d"}
	require.NoError(t, db.Create(&j).Error)
	m := models.Moment{JourneyID: j.ID, DayID: "d", UploaderID: "alice", Kind: "photo", Status: models.MomentReady}
	require.NoError(t, db.Create(&m).Error)
	key := storage.MomentKey(j.ID, m.ID, "full.jpg")
	require.NoError(t, store.Put(ctx, key, "image/jpeg", strings.NewReader("x")))

	svc := services.NewMomentService(services.MomentDeps{
		Access:  services.NewAccessService(repository.NewJourneyRepository(db), repository.NewMemberRepository(db)),
		Moments: repository.NewMomentRepository(db), Store: store})
	require.NoError(t, admin.DeleteMoment(ctx, svc, m.ID))
	var n int64
	db.Model(&models.Moment{}).Where("id = ?", m.ID).Count(&n)
	assert.Zero(t, n)
	assert.ErrorIs(t, admin.DeleteMoment(ctx, svc, m.ID), services.ErrMomentNotFound)
}
