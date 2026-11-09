package services

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/media"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/storage"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeVideo struct {
	calls int
	err   error
}

func (f *fakeVideo) Process(_ context.Context, inPath, outDir string) (*media.VideoResult, error) {
	f.calls++
	if _, err := os.Stat(inPath); err != nil {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	p := filepath.Join(outDir, "video.mp4")
	if err := os.WriteFile(p, []byte("fake-mp4"), 0o600); err != nil {
		return nil, err
	}
	return &media.VideoResult{Path: p, Thumb: []byte("fake-thumb"), Width: 640, Height: 360, DurationMs: 5000}, nil
}

type momentEnv struct {
	t     *testing.T
	db    *gorm.DB
	svc   *MomentService
	store *storage.MemoryStore
	video *fakeVideo
	temp  string
	now   time.Time
	repo  repository.MomentRepository
}

func newMomentEnv(t *testing.T) *momentEnv {
	db := testutil.NewTestDB(t)
	e := &momentEnv{t: t, db: db, store: storage.NewMemoryStore(), video: &fakeVideo{},
		temp: t.TempDir(), now: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
	journeys := repository.NewJourneyRepository(db)
	members := repository.NewMemberRepository(db)
	e.repo = repository.NewMomentRepository(db)
	follows := repository.NewFollowRepository(db)
	access := NewAccessService(journeys, members).WithFollows(follows)
	privacy := NewPrivacyService(PrivacyDeps{Access: access, Follows: follows, Members: members,
		Users: repository.NewUserRepository(db), Now: func() time.Time { return e.now }})
	e.svc = NewMomentService(MomentDeps{
		Access: access, Privacy: privacy, Moments: e.repo, Itinerary: repository.NewItineraryRepository(db),
		Users: repository.NewUserRepository(db), Store: e.store, Video: e.video,
		TempDir: e.temp, Now: func() time.Time { return e.now },
	})
	return e
}

func (e *momentEnv) user(name string) {
	require.NoError(e.t, e.db.Create(&models.User{ID: name, Email: name + "@x.com", Username: name, DisplayName: name}).Error)
}

func (e *momentEnv) journey(ownerID string, membersCanPost bool, visibility string) *models.Journey {
	j := &models.Journey{OwnerID: ownerID, Title: "Trip", Destination: "Nagoya",
		DepartureDate: e.now, ReturnDate: e.now.Add(24 * time.Hour), MembersCanPost: true}
	if visibility != "" {
		j.Visibility = visibility
	}
	require.NoError(e.t, e.db.Create(j).Error)
	if !membersCanPost {
		require.NoError(e.t, e.db.Model(j).Update("members_can_post", false).Error)
		j.MembersCanPost = false
	}
	return j
}

func (e *momentEnv) member(j *models.Journey, userID, role string) {
	require.NoError(e.t, e.db.Create(&models.JourneyMember{JourneyID: j.ID, UserID: userID, Role: role, Status: models.StatusJoined}).Error)
}

func (e *momentEnv) day(j *models.Journey) *models.ItineraryDay {
	d := &models.ItineraryDay{JourneyID: j.ID, Date: "d", DateISO: e.now}
	require.NoError(e.t, e.db.Create(d).Error)
	return d
}

func (e *momentEnv) plan(d *models.ItineraryDay) *models.ActivityPlan {
	p := &models.ActivityPlan{ItineraryDayID: d.ID, Description: "x"}
	require.NoError(e.t, e.db.Create(p).Error)
	return p
}

func photoIn(dayID string) StartUploadInput {
	return StartUploadInput{Kind: "photo", ContentType: "image/jpeg", SizeBytes: 1000, DayID: dayID}
}

func (e *momentEnv) seed(j *models.Journey, d *models.ItineraryDay, uploader, status string, n int, at time.Time) {
	for i := 0; i < n; i++ {
		require.NoError(e.t, e.db.Create(&models.Moment{JourneyID: j.ID, DayID: d.ID, UploaderID: uploader,
			Kind: "photo", Status: status, CreatedAt: at}).Error)
	}
}

func jpegWithExif(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for x := 0; x < 64; x++ {
		for y := 0; y < 48; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), 100, 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	b := buf.Bytes()
	payload := append([]byte("Exif\x00\x00"), []byte("GPS-SECRET-LOCATION")...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte((len(payload) + 2) & 0xFF)}
	seg = append(seg, payload...)
	out := append([]byte{}, b[:2]...)
	out = append(out, seg...)
	return append(out, b[2:]...)
}

func TestStartUpload_MemberCanPost(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	e.user("bob")
	j := e.journey("alice", true, "")
	e.member(j, "bob", models.RoleTraveler)
	d := e.day(j)
	p := e.plan(d)
	in := photoIn(d.ID)
	in.PlanID = p.ID
	in.Caption = "  hi  "

	res, err := e.svc.StartUpload(context.Background(), "bob", j.Slug, in)
	require.NoError(t, err)
	assert.Equal(t, models.MomentPending, res.Moment.Status)
	assert.Equal(t, "hi", res.Moment.Caption)
	assert.Empty(t, res.Moment.URL)
	assert.Empty(t, res.Moment.ThumbURL)
	assert.True(t, res.Moment.CanDelete)
	assert.Equal(t, "bob", res.Moment.Uploader.Username)
	row, err := e.repo.Get(context.Background(), res.Moment.ID)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", row.ContentType)
	assert.EqualValues(t, 1000, row.SizeBytes)
}

func TestStartUpload_TravelerBlockedWhenMembersCanPostFalse(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	e.user("bob")
	j := e.journey("alice", false, "")
	e.member(j, "bob", models.RoleTraveler)
	d := e.day(j)

	_, err := e.svc.StartUpload(context.Background(), "bob", j.Slug, photoIn(d.ID))
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = e.svc.StartUpload(context.Background(), "alice", j.Slug, photoIn(d.ID))
	assert.NoError(t, err)
}

func TestStartUpload_StrangerOnPrivateTripIsNotFound(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	e.user("eve")
	j := e.journey("alice", true, models.VisibilityPrivate)
	d := e.day(j)

	_, err := e.svc.StartUpload(context.Background(), "eve", j.Slug, photoIn(d.ID))
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.StartUpload(context.Background(), "eve", "nope", photoIn(d.ID))
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestStartUpload_LegacyTripForbidden(t *testing.T) {
	e := newMomentEnv(t)
	e.user("eve")
	j := e.journey("", true, models.VisibilityPublic)
	d := e.day(j)

	_, err := e.svc.StartUpload(context.Background(), "eve", j.Slug, photoIn(d.ID))
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestStartUpload_ValidatesKindTypeSizeDayPlanCaption(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	other := e.journey("alice", true, "")
	d := e.day(j)
	otherDay := e.day(other)
	otherPlan := e.plan(e.day(j))

	cases := []struct {
		name   string
		mutate func(in *StartUploadInput)
		want   error // nil means any ValidationError
	}{
		{"kind", func(in *StartUploadInput) { in.Kind = "gif" }, nil},
		{"heic", func(in *StartUploadInput) { in.ContentType = "image/heic" }, nil},
		{"video type for photo", func(in *StartUploadInput) { in.ContentType = "video/mp4" }, nil},
		{"size 0", func(in *StartUploadInput) { in.SizeBytes = 0 }, nil},
		{"photo too big", func(in *StartUploadInput) { in.SizeBytes = 10485761 }, nil},
		{"video too big", func(in *StartUploadInput) { in.Kind, in.ContentType, in.SizeBytes = "video", "video/mp4", 104857601 }, nil},
		{"other trip day", func(in *StartUploadInput) { in.DayID = otherDay.ID }, ErrDayNotFound},
		{"plan of other day", func(in *StartUploadInput) { in.PlanID = otherPlan.ID }, ErrPlanNotFound},
		{"caption 301 runes", func(in *StartUploadInput) { in.Caption = strings.Repeat("ก", 301) }, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := photoIn(d.ID)
			c.mutate(&in)
			_, err := e.svc.StartUpload(context.Background(), "alice", j.Slug, in)
			if c.want != nil {
				assert.ErrorIs(t, err, c.want)
				return
			}
			var ve *ValidationError
			assert.ErrorAs(t, err, &ve)
		})
	}

	in := photoIn(d.ID)
	in.Caption = strings.Repeat("ก", 300)
	_, err := e.svc.StartUpload(context.Background(), "alice", j.Slug, in)
	assert.NoError(t, err)
	in = StartUploadInput{Kind: "video", ContentType: "video/quicktime", SizeBytes: 104857600, DayID: d.ID}
	_, err = e.svc.StartUpload(context.Background(), "alice", j.Slug, in)
	assert.NoError(t, err)
}

func TestStartUpload_TripLimit(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	e.seed(j, d, "someone", models.MomentReady, 499, e.now.Add(-48*time.Hour))
	_, err := e.svc.StartUpload(context.Background(), "alice", j.Slug, photoIn(d.ID))
	require.NoError(t, err)
	_, err = e.svc.StartUpload(context.Background(), "alice", j.Slug, photoIn(d.ID))
	assert.ErrorIs(t, err, ErrTripMomentLimit)
}

func TestStartUpload_RateLimits(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)

	e.seed(j, d, "alice", models.MomentReady, 100, e.now.Add(-10*time.Minute))
	_, err := e.svc.StartUpload(context.Background(), "alice", j.Slug, photoIn(d.ID))
	assert.ErrorIs(t, err, ErrTooManyRequests)

}

func TestStartUpload_RateLimitIgnoresOldStarts(t *testing.T) {
	e2 := newMomentEnv(t)
	e2.user("alice")
	j2 := e2.journey("alice", true, "")
	d2 := e2.day(j2)
	e2.seed(j2, d2, "alice", models.MomentReady, 100, e2.now.Add(-2*time.Hour))
	_, err := e2.svc.StartUpload(context.Background(), "alice", j2.Slug, photoIn(d2.ID))
	assert.NoError(t, err)
}

func TestStartUpload_PendingLimit(t *testing.T) {
	e3 := newMomentEnv(t)
	e3.user("alice")
	j3 := e3.journey("alice", true, "")
	d3 := e3.day(j3)
	e3.seed(j3, d3, "alice", models.MomentPending, 40, e3.now.Add(-3*time.Hour))
	_, err := e3.svc.StartUpload(context.Background(), "alice", j3.Slug, photoIn(d3.ID))
	assert.ErrorIs(t, err, ErrTooManyRequests)
}

func TestStartUpload_ReturnsRawUploadTarget(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)

	res, err := e.svc.StartUpload(context.Background(), "alice", j.Slug, photoIn(d.ID))
	require.NoError(t, err)
	assert.Contains(t, res.Upload.URL, "uploads/"+j.ID+"/"+res.Moment.ID)
	assert.Equal(t, "0,10485760", res.Upload.Headers["x-goog-content-length-range"])
	assert.Equal(t, "image/jpeg", res.Upload.Headers["Content-Type"])
	assert.WithinDuration(t, time.Now().Add(15*time.Minute), res.Upload.ExpiresAt, time.Minute)

	v, err := e.svc.StartUpload(context.Background(), "alice", j.Slug,
		StartUploadInput{Kind: "video", ContentType: "video/mp4", SizeBytes: 5, DayID: d.ID})
	require.NoError(t, err)
	assert.Equal(t, "0,104857600", v.Upload.Headers["x-goog-content-length-range"])
}

func (e *momentEnv) start(userID string, j *models.Journey, in StartUploadInput) string {
	res, err := e.svc.StartUpload(context.Background(), userID, j.Slug, in)
	require.NoError(e.t, err)
	return res.Moment.ID
}

func TestComplete_PhotoHappyPath(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	id := e.start("alice", j, photoIn(d.ID))
	e.store.PutRaw(storage.RawKey(j.ID, id), jpegWithExif(t))

	v, err := e.svc.Complete(context.Background(), "alice", j.Slug, id)
	require.NoError(t, err)
	assert.Equal(t, models.MomentReady, v.Status)
	assert.Contains(t, v.URL, "moments/"+j.ID+"/"+id+"/full.jpg")
	assert.Contains(t, v.ThumbURL, "moments/"+j.ID+"/"+id+"/thumb.jpg")
	assert.Equal(t, 64, v.Width)
	assert.Equal(t, 48, v.Height)
	for _, k := range e.store.Keys() {
		assert.False(t, strings.HasPrefix(k, "uploads/"), "raw object left behind: %s", k)
	}
	rc, size, err := e.store.Open(context.Background(), storage.MomentKey(j.ID, id, "full.jpg"))
	require.NoError(t, err)
	defer rc.Close()
	var buf bytes.Buffer
	_, err = buf.ReadFrom(rc)
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "Exif")
	assert.NotContains(t, buf.String(), "GPS-SECRET")
	row, _ := e.repo.Get(context.Background(), id)
	assert.Equal(t, size, row.SizeBytes)
}

func TestComplete_RejectsDisguisedFile(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	id := e.start("alice", j, photoIn(d.ID))
	e.store.PutRaw(storage.RawKey(j.ID, id), []byte("<html><script>alert(1)</script></html>"))

	_, err := e.svc.Complete(context.Background(), "alice", j.Slug, id)
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, media.ErrUnsupportedFormat.Error(), ve.Message)
	row, _ := e.repo.Get(context.Background(), id)
	require.NotNil(t, row)
	assert.Equal(t, models.MomentRejected, row.Status)
	assert.Empty(t, e.store.Keys())
	_, err = e.svc.Complete(context.Background(), "alice", j.Slug, id)
	assert.ErrorIs(t, err, ErrMomentNotFound)
}

func TestComplete_RejectsOversizeActualFile(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	id := e.start("alice", j, photoIn(d.ID))
	e.store.PutRaw(storage.RawKey(j.ID, id), make([]byte, 10485761))

	_, err := e.svc.Complete(context.Background(), "alice", j.Slug, id)
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "file too large", ve.Message)
	row, _ := e.repo.Get(context.Background(), id)
	require.NotNil(t, row)
	assert.Equal(t, models.MomentRejected, row.Status)
	assert.Empty(t, e.store.Keys())
	_, err = e.svc.Complete(context.Background(), "alice", j.Slug, id)
	assert.ErrorIs(t, err, ErrMomentNotFound)
}

func TestComplete_OtherUserCannotComplete(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	e.user("bob")
	j := e.journey("alice", true, "")
	e.member(j, "bob", models.RoleTraveler)
	d := e.day(j)
	id := e.start("alice", j, photoIn(d.ID))
	e.store.PutRaw(storage.RawKey(j.ID, id), jpegWithExif(t))

	_, err := e.svc.Complete(context.Background(), "bob", j.Slug, id)
	assert.ErrorIs(t, err, ErrMomentNotFound)
	_, err = e.svc.Complete(context.Background(), "alice", j.Slug, "nope")
	assert.ErrorIs(t, err, ErrMomentNotFound)
}

func TestComplete_OtherTripMomentNotFound(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	j2 := e.journey("alice", true, "")
	d := e.day(j)
	id := e.start("alice", j, photoIn(d.ID))
	_, err := e.svc.Complete(context.Background(), "alice", j2.Slug, id)
	assert.ErrorIs(t, err, ErrMomentNotFound)
}

func TestComplete_CompleteTwice(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	id := e.start("alice", j, photoIn(d.ID))
	e.store.PutRaw(storage.RawKey(j.ID, id), jpegWithExif(t))
	_, err := e.svc.Complete(context.Background(), "alice", j.Slug, id)
	require.NoError(t, err)
	_, err = e.svc.Complete(context.Background(), "alice", j.Slug, id)
	assert.ErrorIs(t, err, ErrMomentNotFound)
}

func TestComplete_MissingUpload(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	id := e.start("alice", j, photoIn(d.ID))

	_, err := e.svc.Complete(context.Background(), "alice", j.Slug, id)
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "upload not found", ve.Message)
	row, _ := e.repo.Get(context.Background(), id)
	require.NotNil(t, row)

	e.store.PutRaw(storage.RawKey(j.ID, id), jpegWithExif(t))
	_, err = e.svc.Complete(context.Background(), "alice", j.Slug, id)
	assert.NoError(t, err)
}

func TestComplete_VideoUsesProcessorAndCleansTemp(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	id := e.start("alice", j, StartUploadInput{Kind: "video", ContentType: "video/mp4", SizeBytes: 100, DayID: d.ID})
	e.store.PutRaw(storage.RawKey(j.ID, id), []byte("raw-video-bytes"))

	v, err := e.svc.Complete(context.Background(), "alice", j.Slug, id)
	require.NoError(t, err)
	assert.Equal(t, 1, e.video.calls)
	assert.Equal(t, models.MomentReady, v.Status)
	assert.EqualValues(t, 5000, v.DurationMs)
	assert.Contains(t, v.URL, "moments/"+j.ID+"/"+id+"/video.mp4")
	keys := e.store.Keys()
	assert.Contains(t, keys, storage.MomentKey(j.ID, id, "video.mp4"))
	assert.Contains(t, keys, storage.MomentKey(j.ID, id, "thumb.jpg"))
	for _, k := range keys {
		assert.False(t, strings.HasPrefix(k, "uploads/"))
	}
	entries, err := os.ReadDir(e.temp)
	require.NoError(t, err)
	assert.Empty(t, entries, fmt.Sprint(entries))
	row, _ := e.repo.Get(context.Background(), id)
	assert.EqualValues(t, len("fake-mp4"), row.SizeBytes)
}

func TestComplete_VideoRejectedByProcessorUsesSentinelMessage(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	e.video.err = fmt.Errorf("ffprobe /srv/tmp/x: %w", media.ErrVideoTooLong)
	id := e.start("alice", j, StartUploadInput{Kind: "video", ContentType: "video/mp4", SizeBytes: 100, DayID: d.ID})
	e.store.PutRaw(storage.RawKey(j.ID, id), []byte("raw"))

	_, err := e.svc.Complete(context.Background(), "alice", j.Slug, id)
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, media.ErrVideoTooLong.Error(), ve.Message)
	assert.Empty(t, e.store.Keys())
	row, _ := e.repo.Get(context.Background(), id)
	require.NotNil(t, row)
	assert.Equal(t, models.MomentRejected, row.Status)
}

func TestComplete_VideoUnavailableKeepsRow(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	id := e.start("alice", j, StartUploadInput{Kind: "video", ContentType: "video/mp4", SizeBytes: 100, DayID: d.ID})
	e.store.PutRaw(storage.RawKey(j.ID, id), []byte("raw"))

	e.svc.Video = nil // processor went away after the upload started
	_, err := e.svc.Complete(context.Background(), "alice", j.Slug, id)
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "video processing is unavailable", ve.Message)
	row, _ := e.repo.Get(context.Background(), id)
	assert.NotNil(t, row)
	assert.Contains(t, e.store.Keys(), storage.RawKey(j.ID, id))
}

func TestComplete_RejectedStillCountsTowardRateLimit(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	e.seed(j, d, "alice", models.MomentRejected, 100, e.now.Add(-time.Minute))
	_, err := e.svc.StartUpload(context.Background(), "alice", j.Slug, photoIn(d.ID))
	assert.ErrorIs(t, err, ErrTooManyRequests)
}

func TestComplete_RecheckPostingRight(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	e.user("bob")
	j := e.journey("alice", true, "")
	e.member(j, "bob", models.RoleTraveler)
	d := e.day(j)
	id := e.start("bob", j, photoIn(d.ID))
	e.store.PutRaw(storage.RawKey(j.ID, id), jpegWithExif(t))
	require.NoError(t, e.db.Model(&models.Journey{}).Where("id = ?", j.ID).Update("members_can_post", false).Error)

	_, err := e.svc.Complete(context.Background(), "bob", j.Slug, id)
	assert.ErrorIs(t, err, ErrForbidden)
	row, _ := e.repo.Get(context.Background(), id)
	require.NotNil(t, row)
	assert.Equal(t, models.MomentPending, row.Status)
	assert.Equal(t, []string{storage.RawKey(j.ID, id)}, e.store.Keys())
}

func TestStartUpload_VideoUnavailableIssuesNoUpload(t *testing.T) {
	e := newMomentEnv(t)
	e.svc.Video = nil
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	_, err := e.svc.StartUpload(context.Background(), "alice", j.Slug,
		StartUploadInput{Kind: "video", ContentType: "video/mp4", SizeBytes: 100, DayID: d.ID})
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "video processing is unavailable", ve.Message)
	var n int64
	require.NoError(t, e.db.Model(&models.Moment{}).Count(&n).Error)
	assert.Equal(t, int64(0), n)
}

// semStore records whether the image semaphore was held when Open ran.
type semStore struct {
	*storage.MemoryStore
	svc      *MomentService
	heldOnOp int
}

func (s *semStore) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	s.heldOnOp = len(s.svc.imageSem)
	return s.MemoryStore.Open(ctx, key)
}

func TestComplete_HoldsSemaphoreBeforeOpeningRawObject(t *testing.T) {
	e := newMomentEnv(t)
	ss := &semStore{MemoryStore: e.store, svc: e.svc}
	e.svc.Store = ss
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	id := e.start("alice", j, photoIn(d.ID))
	e.store.PutRaw(storage.RawKey(j.ID, id), jpegWithExif(t))
	_, err := e.svc.Complete(context.Background(), "alice", j.Slug, id)
	require.NoError(t, err)
	assert.Equal(t, 1, ss.heldOnOp)
}
