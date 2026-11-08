# Trip Members & Granular Editing Implementation Plan (MVP แผน 2 จาก 6)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ให้ทริปมีคนร่วมทริปได้ (เชิญด้วย `@username` หรือลิงก์เชิญ, ต้องกดรับก่อน, มี role) และแก้ทริปทีละวัน/ทีละกิจกรรมได้ โดยหลายคนแก้พร้อมกันไม่ทับกัน

**Architecture:** ต่อจากแผน 1 (layer Handler → Service → Repository, `internal/app.New` ประกอบทุกอย่าง) เจ้าของทริปยังมาจาก `journeys.owner_id` ส่วนตาราง `journey_members` เก็บเฉพาะ editor/traveler สิทธิ์ทุกอย่างผ่านจุดเดียวคือ `AccessService` ส่วน endpoint แก้ทีละส่วนอยู่ใน `ItineraryService` แยกจาก `JourneyService` เดิม

**Tech Stack:** เหมือนแผน 1 (Go 1.24.1, Fiber v2, GORM, PostgreSQL, SQLite in-memory ใน test)

**Spec:** `docs/business-requirements/2026-11-08-social-trip-sharing.md` (ข้อ 6.2 ข้อ 5–6, 7.3, 7.4)

**Base:** branch `feature/trip-members` แตกจาก `feature/users-and-login` (แผน 1 ยังไม่ merge)

## Global Constraints

- ทุก response ใช้ `utils.Success` / `utils.Error`; error ทั้งหมดผ่าน `respondError` ใน `internal/handlers/errors.go`
- ID ใหม่ใช้ `newShortID()`; token ลิงก์เชิญเก็บแค่ `auth.HashToken` ไม่เก็บ token ดิบ
- Role ของสมาชิก: `editor` หรือ `traveler` เท่านั้น (owner มาจาก `journeys.owner_id`)
- Status ของสมาชิก: `invited` หรือ `joined` (ปฏิเสธ / ออก / ถูกเอาออก = ลบแถวทิ้ง)
- ทริปละไม่เกิน **20 คน** นับ owner + joined + invited
- ลิงก์เชิญอายุ **7 วัน**, owner ยกเลิกได้, URL = `{APP_WEB_URL}/join/{token}`
- ทริปเก่าที่ไม่มีเจ้าของ (`owner_id` ว่าง) ยัง read-only ทั้งหมด: เชิญไม่ได้ แก้ไม่ได้
- `cover_url` ต้องเป็น `https://` มี host และยาวไม่เกิน 2048 byte (กฎเดียวกับ avatar)
- `go.mod` คง `go 1.24.1`; ห้าม commit `.env`

## Review Focus

1. **Full update (`PUT /journeys/:slug/update`) กับ ID ของ Day/Plan ที่ client ส่งมา** เมื่อเปิด `id` ใน JSON แล้ว client อาจส่ง ID ของ day/plan ของทริปอื่นมาใน body → service ต้องล้าง ID ของ day/plan ทุกตัวก่อนบันทึก → test ใน Task 1
2. **Day/Plan ID ข้ามทริป** `PATCH /journeys/A/plans/<planId ของทริป B>` ต้องได้ 404 ไม่ใช่แก้ของทริป B → test ใน Task 5
3. **คนที่ยังไม่กดรับคำเชิญ (`invited`) ต้องไม่มีสิทธิ์อะไรเลย** และไม่โผล่ในรายชื่อสาธารณะของทริป → test ใน Task 3 และ 6
4. **ทริปเต็ม 20 คน** การเชิญคนที่ 21 และการ join ผ่านลิงก์เมื่อเต็ม ต้องได้ 409 → test ใน Task 4
5. **Owner ออกจากทริปตัวเองไม่ได้** และ editor เปลี่ยน role ตัวเอง / เอาคนอื่นออกไม่ได้ → test ใน Task 4

---

## File Structure

| ไฟล์ | หน้าที่ |
|---|---|
| `internal/models/member.go` | `JourneyMember`, `InviteLink`, role/status constants, `MemberView` |
| `internal/repository/member_repository.go`, `member_gorm.go` | members + invite links |
| `internal/repository/itinerary_repository.go`, `itinerary_gorm.go` | day/plan CRUD ทีละแถว |
| `internal/services/access_service.go` | หา role ของ user ในทริป + ตรวจสิทธิ์ |
| `internal/services/member_service.go` | เชิญ, รับ/ปฏิเสธ, ออก, เอาออก, เปลี่ยน role, ลิงก์เชิญ |
| `internal/services/itinerary_service.go` | แก้ข้อมูลทริป, day/plan CRUD, ย้าย, เรียงลำดับ |
| `internal/services/validate.go` | `validHTTPSURL` ใช้ร่วมกับ avatar |
| `internal/handlers/member_handler.go`, `itinerary_handler.go` | HTTP |
| แก้ `internal/models/journey.go`, `internal/services/journey_service.go`, `internal/services/user_service.go`, `internal/repository/journey_gorm.go`, `internal/handlers/errors.go`, `internal/handlers/journey_handler.go`, `internal/routes/routes.go`, `internal/app/app.go`, `database/postgres.go`, `README.md` | |

---

### Task 1: เปิด ID ของ Day/Plan + ลำดับ Plan + ปิดช่อง full update

**Files:**
- Modify: `internal/models/journey.go`, `internal/repository/journey_gorm.go`, `internal/services/journey_service.go`
- Test: `internal/app/journey_http_test.go`, `internal/repository/journey_gorm_test.go`

**Interfaces:**
- Produces:
  - `ItineraryDay.ID` json `"id"`; `ActivityPlan.ID` json `"id"`; `ActivityPlan.ItineraryDayID` json `"day_id"`
  - `ActivityPlan.Position int` — `gorm:"not null;default:0" json:"position"`
  - `Journey.CoverURL string` json `"cover_url"`; `Journey.EditorsCanInvite bool` — `gorm:"not null;default:false" json:"editors_can_invite"`
  - `GetBySlug` เรียง plan ด้วย `position ASC, time ASC`
  - `JourneyService.Create` และ `Update` ล้าง `ID` ของทุก day และทุก plan (และ `ItineraryDayID`) ก่อนบันทึก; `Create` ยังตั้ง `Position` ของ plan ตามลำดับใน body (0,1,2,…) ต่อวัน; `Update` ก็เช่นกัน

- [ ] **Step 1: Write failing tests**
  - `TestJourneyRepository_PlanOrderByPosition`: plan `time "09:00" position 1` กับ `time "18:00" position 0` → ได้ `"18:00"` ก่อน; ถ้า position เท่ากันทั้งคู่ เรียงตาม time
  - `TestHTTP_GetJourneyExposesIDs`: `GET /journeys/:slug` → ทุก day มี `id` ยาว 10, ทุก plan มี `id` และ `day_id` ตรงกับ day
  - `TestHTTP_UpdateIgnoresClientChildIDs` (Review Focus 1): ทริป A ของ alice, ทริป B ของ bob; alice `PUT` ทริป A โดยใส่ `id` ของ day และ plan ของทริป B ใน body → 200; `GET` ทริป B ยังเหมือนเดิมทุก field; day ของทริป A มี `id` ใหม่ที่ไม่ซ้ำกับของ B
  - `TestHTTP_CreateSetsPositions`: สร้างทริป 1 วัน 3 plan → `position` เป็น 0,1,2 ตามลำดับใน body
- [ ] **Step 2:** Run → FAIL
- [ ] **Step 3:** Implement
- [ ] **Step 4:** Run `go test ./...` → PASS
- [ ] **Step 5: Commit** `feat: expose itinerary ids and plan positions`

---

### Task 2: Member models + repository

**Files:**
- Create: `internal/models/member.go`, `internal/repository/member_repository.go`, `internal/repository/member_gorm.go`
- Modify: `database/postgres.go` (`Models()` เพิ่ม `JourneyMember`, `InviteLink`)
- Test: `internal/repository/member_gorm_test.go`

**Interfaces:**
- Produces:
  ```go
  const (
      RoleOwner    = "owner"     // ใช้ใน response เท่านั้น ไม่เก็บใน DB
      RoleEditor   = "editor"
      RoleTraveler = "traveler"
      StatusInvited = "invited"
      StatusJoined  = "joined"
  )
  type JourneyMember struct {
      ID        string    `gorm:"type:varchar(10);primaryKey" json:"id"`
      JourneyID string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_member_journey_user" json:"-"`
      UserID    string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_member_journey_user;index" json:"-"`
      Role      string    `gorm:"type:varchar(10);not null" json:"role"`
      Status    string    `gorm:"type:varchar(10);not null" json:"status"`
      InvitedBy string    `gorm:"type:varchar(10)" json:"-"`
      CreatedAt time.Time `json:"created_at"`
      UpdatedAt time.Time `json:"updated_at"`
  }
  type InviteLink struct {
      ID        string     `gorm:"type:varchar(10);primaryKey" json:"id"`
      JourneyID string     `gorm:"type:varchar(10);not null;index" json:"-"`
      TokenHash string     `gorm:"type:char(64);uniqueIndex;not null" json:"-"`
      Role      string     `gorm:"type:varchar(10);not null" json:"role"`
      CreatedBy string     `gorm:"type:varchar(10);not null" json:"-"`
      ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
      RevokedAt *time.Time `json:"revoked_at,omitempty"`
      CreatedAt time.Time  `json:"created_at"`
  }
  type MemberView struct {   // รายชื่อที่ส่งออก API
      Username, DisplayName, AvatarURL, Role, Status string // json: username, display_name, avatar_url, role, status
  }
  type MemberRepository interface {
      Create(ctx context.Context, m *models.JourneyMember) error
      Get(ctx context.Context, journeyID, userID string) (*models.JourneyMember, error)   // nil,nil ถ้าไม่มี
      GetByID(ctx context.Context, id string) (*models.JourneyMember, error)
      Update(ctx context.Context, m *models.JourneyMember) error
      Delete(ctx context.Context, id string) error
      ListByJourney(ctx context.Context, journeyID string) ([]models.MemberView, error)   // join users, เรียง joined ก่อน แล้วตาม created_at
      ListInvitesForUser(ctx context.Context, userID string) ([]models.JourneyMember, error) // status invited
      ListJourneyIDsForUser(ctx context.Context, userID string) ([]string, error)          // status joined
      CountActive(ctx context.Context, journeyID string) (int64, error)                     // invited + joined
      CreateInviteLink(ctx context.Context, l *models.InviteLink) error
      GetInviteLinkByHash(ctx context.Context, hash string) (*models.InviteLink, error)
      GetInviteLink(ctx context.Context, journeyID, id string) (*models.InviteLink, error)
      RevokeInviteLink(ctx context.Context, id string, at time.Time) error
      ListInviteLinks(ctx context.Context, journeyID string) ([]models.InviteLink, error)   // ยังไม่ revoke
  }
  func NewMemberRepository(db *gorm.DB) MemberRepository
  ```

- [ ] **Step 1: Write failing tests**
  - `TestMemberRepository_UniquePerJourney`: user เดียวกันเข้าทริปเดียวกันสองแถว → error; คนละทริป → ได้
  - `TestMemberRepository_ListByJourney`: 1 joined editor + 1 invited traveler → 2 แถว, joined มาก่อน, มี `username` จากตาราง users
  - `TestMemberRepository_CountActive`: 2 แถว → 2; ลบ 1 → 1
  - `TestMemberRepository_InviteLinks`: สร้าง 2 ลิงก์, revoke 1 → `ListInviteLinks` ได้ 1; `GetInviteLinkByHash` ไม่เจอ → nil,nil
- [ ] **Step 2:** Run → FAIL
- [ ] **Step 3:** Implement
- [ ] **Step 4:** Run → PASS
- [ ] **Step 5: Commit** `feat: add journey member and invite link models`

---

### Task 3: AccessService (สิทธิ์ในทริป)

**Files:**
- Create: `internal/services/access_service.go`
- Test: `internal/services/access_service_test.go`

**Interfaces:**
- Consumes: `JourneyRepository.GetBySlug`, `MemberRepository.Get`
- Produces:
  ```go
  type Access struct {
      Journey *models.Journey
      Role    string // "owner" | "editor" | "traveler" | "" (ไม่ใช่สมาชิก หรือยังเป็น invited)
  }
  func NewAccessService(j repository.JourneyRepository, m repository.MemberRepository) *AccessService
  func (s *AccessService) Load(ctx context.Context, slug, userID string) (*Access, error) // ErrNotFound ถ้าไม่มีทริป; userID ว่างได้
  func (a *Access) CanEdit() bool      // owner หรือ editor
  func (a *Access) CanInvite() bool    // owner หรือ (editor && Journey.EditorsCanInvite)
  func (a *Access) IsOwner() bool
  func (a *Access) IsMember() bool     // Role != ""
  ```
- ทริปที่ `OwnerID == ""`: ทุกฟังก์ชัน `Can*` คืน false เสมอ
- สมาชิกที่ status `invited` → `Role == ""`

- [ ] **Step 1: Write failing tests** (table test `TestAccess`)
  - owner → CanEdit ✓ CanInvite ✓ IsOwner ✓
  - joined editor → CanEdit ✓, CanInvite ✗ เมื่อ `EditorsCanInvite=false`, ✓ เมื่อ true
  - joined traveler → CanEdit ✗ CanInvite ✗ IsMember ✓
  - invited editor (Review Focus 3) → ทุกอย่าง ✗, `Role == ""`
  - คนนอก / userID ว่าง → ทุกอย่าง ✗
  - legacy ทริป (owner ว่าง) + userID ว่าง → CanEdit ✗ (ห้ามให้ "" == "" ผ่าน)
  - slug ไม่มี → `ErrNotFound`
- [ ] **Step 2:** Run → FAIL
- [ ] **Step 3:** Implement
- [ ] **Step 4:** Run → PASS
- [ ] **Step 5: Commit** `feat: add trip access service`

---

### Task 4: MemberService

**Files:**
- Create: `internal/services/member_service.go`
- Test: `internal/services/member_service_test.go`

**Interfaces:**
- Consumes: `AccessService` (Task 3), `MemberRepository` (Task 2), `UserRepository.GetByUsername`, `auth.NewToken`, `auth.HashToken`
- Produces:
  ```go
  const MaxTripPeople = 20
  const InviteLinkTTL = 7 * 24 * time.Hour
  var ErrMemberNotFound = errors.New("member not found")
  var ErrInviteNotFound = errors.New("invite not found")
  var ErrAlreadyMember  = errors.New("already a member")
  var ErrTripFull       = errors.New("trip is full")
  type MemberDeps struct { Access *AccessService; Members repository.MemberRepository; Users repository.UserRepository; WebURL string; Now func() time.Time }
  func NewMemberService(d MemberDeps) *MemberService
  func (s *MemberService) Invite(ctx context.Context, actorID, slug, username, role string) (*models.JourneyMember, error)
  func (s *MemberService) ListMembers(ctx context.Context, actorID, slug string) ([]models.MemberView, error)       // owner แถวแรก + joined + invited; เฉพาะสมาชิกเรียกได้
  func (s *MemberService) PublicMembers(ctx context.Context, slug string) ([]models.MemberView, error)              // owner + joined เท่านั้น
  func (s *MemberService) MyInvites(ctx context.Context, userID string) ([]InviteView, error)                       // InviteView{ID, JourneySlug, JourneyTitle, Role, InvitedBy (username)}
  func (s *MemberService) RespondInvite(ctx context.Context, userID, memberID string, accept bool) error            // accept → joined; decline → ลบแถว
  func (s *MemberService) Leave(ctx context.Context, userID, slug string) error
  func (s *MemberService) Remove(ctx context.Context, actorID, slug, username string) error                        // owner เท่านั้น
  func (s *MemberService) ChangeRole(ctx context.Context, actorID, slug, username, role string) error              // owner เท่านั้น
  func (s *MemberService) CreateInviteLink(ctx context.Context, actorID, slug, role string) (url string, link *models.InviteLink, err error)
  func (s *MemberService) RevokeInviteLink(ctx context.Context, actorID, slug, linkID string) error               // owner เท่านั้น
  func (s *MemberService) ListInviteLinks(ctx context.Context, actorID, slug string) ([]models.InviteLink, error) // คนที่ CanInvite
  func (s *MemberService) JoinByLink(ctx context.Context, userID, token string) (slug string, err error)           // คลิกลิงก์เอง = ยินยอม → joined ทันที
  ```
- กฎ:
  - `role` ต้องเป็น `editor` หรือ `traveler` อย่างอื่น → `ValidationError`
  - Invite / CreateInviteLink / ListInviteLinks ต้อง `CanInvite()` ไม่งั้น `ErrForbidden`; editor ที่เชิญได้ เชิญได้แค่ role `traveler` (ขอ `editor` → `ErrForbidden`)
  - เชิญตัวเอง / เชิญ owner / คนที่มีแถวอยู่แล้ว → `ErrAlreadyMember`; username ไม่มี → `ErrUserNotFound`
  - ก่อนเพิ่มคน: `1 + CountActive >= MaxTripPeople` → `ErrTripFull`
  - `RespondInvite`: แถวต้องเป็นของ `userID` และ status `invited` ไม่งั้น `ErrInviteNotFound`
  - `Leave`: owner → `ValidationError("owner cannot leave the trip")`; ไม่ใช่สมาชิก → `ErrMemberNotFound`
  - `JoinByLink`: ไม่เจอ / หมดอายุ / revoke แล้ว → `ErrInviteNotFound`; ถ้าเป็น owner หรือ joined อยู่แล้ว → คืน slug เฉย ๆ ไม่ error; ถ้ามีแถว `invited` อยู่ → เปลี่ยนเป็น joined; เช็คทริปเต็มก่อนเพิ่มแถวใหม่
  - ลิงก์ที่คืนจาก `CreateInviteLink` = `{WebURL}/join/{token}` (token ดิบออกไปครั้งเดียว ไม่เก็บ)

- [ ] **Step 1: Write failing tests**
  - `TestInvite_ByOwner`: สร้างแถว `invited`; `PublicMembers` ยังไม่มี bob; bob `RespondInvite(accept)` → `PublicMembers` มี bob role ที่เชิญ
  - `TestInvite_Decline`: decline → แถวหาย; owner เชิญใหม่ได้
  - `TestInvite_Permissions`: traveler เชิญ → `ErrForbidden`; editor ตอน `EditorsCanInvite=false` → `ErrForbidden`; ตอน true เชิญ traveler ได้ แต่เชิญ editor → `ErrForbidden`; invited (ยังไม่รับ) เชิญ → `ErrForbidden`
  - `TestInvite_Duplicates`: เชิญ owner, เชิญซ้ำ, เชิญตัวเอง → `ErrAlreadyMember`; username ไม่มี → `ErrUserNotFound`; role `"admin"` → `ValidationError`
  - `TestTripFull` (Review Focus 4): owner + 19 คน → คนที่ 21 → `ErrTripFull`; `JoinByLink` ตอนเต็ม → `ErrTripFull`
  - `TestLeave` (Review Focus 5): owner → `ValidationError`; traveler ออกได้แล้วหายจาก `PublicMembers`; คนนอก → `ErrMemberNotFound`
  - `TestRemoveAndChangeRole` (Review Focus 5): editor เอาคนอื่นออก / เปลี่ยน role ตัวเอง → `ErrForbidden`; owner ทำได้
  - `TestInviteLink`: สร้าง → URL ขึ้นต้น `http://web.test/join/` และ token 64 hex; `JoinByLink` → joined; เลื่อนเวลา 7 วัน → `ErrInviteNotFound`; revoke แล้ว → `ErrInviteNotFound`; owner กดลิงก์ตัวเอง → คืน slug ไม่ error
  - `TestLegacyTrip`: ทริป owner ว่าง → Invite / CreateInviteLink → `ErrForbidden`
- [ ] **Step 2:** Run → FAIL
- [ ] **Step 3:** Implement
- [ ] **Step 4:** Run → PASS
- [ ] **Step 5: Commit** `feat: add trip member service`

---

### Task 5: ItineraryService (แก้ทีละส่วน)

**Files:**
- Create: `internal/services/itinerary_service.go`, `internal/services/validate.go`, `internal/repository/itinerary_repository.go`, `internal/repository/itinerary_gorm.go`
- Modify: `internal/services/user_service.go` (ใช้ `validHTTPSURL` ตัวกลาง), `internal/services/journey_service.go` (Update ใช้ `AccessService`: editor ทำ full update ได้, Delete ยัง owner เท่านั้น)
- Test: `internal/services/itinerary_service_test.go`

**Interfaces:**
- Produces:
  ```go
  var ErrDayNotFound  = errors.New("day not found")
  var ErrPlanNotFound = errors.New("plan not found")
  type JourneyPatch struct { Title, Destination, Country, CoverURL *string; DepartureDate, ReturnDate *time.Time; EditorsCanInvite *bool }
  type DayInput  struct { Date *string; DateISO *time.Time; Title *string }            // json: date, date_iso, title
  type PlanInput struct { Time, Description, Emoji, MapURL *string; DayID *string }     // json: time, description, emoji, map_url, day_id
  func NewItineraryService(a *AccessService, r repository.ItineraryRepository, j repository.JourneyRepository) *ItineraryService
  func (s *ItineraryService) PatchJourney(ctx, userID, slug string, p JourneyPatch) (*models.Journey, error)
  func (s *ItineraryService) AddDay(ctx, userID, slug string, in DayInput) (*models.ItineraryDay, error)
  func (s *ItineraryService) UpdateDay(ctx, userID, slug, dayID string, in DayInput) (*models.ItineraryDay, error)
  func (s *ItineraryService) DeleteDay(ctx, userID, slug, dayID string) error
  func (s *ItineraryService) AddPlan(ctx, userID, slug, dayID string, in PlanInput) (*models.ActivityPlan, error)   // position = max+1 ในวันนั้น
  func (s *ItineraryService) UpdatePlan(ctx, userID, slug, planID string, in PlanInput) (*models.ActivityPlan, error) // DayID ไม่ nil = ย้ายวัน → ไปท้ายวันใหม่
  func (s *ItineraryService) DeletePlan(ctx, userID, slug, planID string) error
  func (s *ItineraryService) ReorderPlans(ctx, userID, slug, dayID string, planIDs []string) error  // ต้องเป็นชุดเดียวกับ plan ในวันนั้นพอดี ไม่งั้น ValidationError; ตั้ง position = index
  ```
  `ItineraryRepository`: `GetDay(ctx, journeyID, dayID)`, `CreateDay`, `UpdateDay`, `DeleteDay`, `GetPlan(ctx, journeyID, planID)` (join ผ่าน day เพื่อยืนยันว่า plan อยู่ในทริปนี้), `CreatePlan`, `UpdatePlan`, `DeletePlan`, `ListPlanIDs(ctx, dayID) ([]string, error)`, `MaxPosition(ctx, dayID) (int, error)`, `SetPositions(ctx, dayID string, ids []string) error` (ใน transaction), `UpdateJourneyFields(ctx, journeyID string, fields map[string]any) error` (อัปเดตเฉพาะ column ที่ส่งมา ไม่ใช้ Save)
  `validHTTPSURL(s string) bool` ใน `validate.go` (ย้ายจาก user_service)
- กฎ:
  - ทุกฟังก์ชันต้อง `CanEdit()` ไม่งั้น `ErrForbidden`; `EditorsCanInvite` ใน patch เปลี่ยนได้เฉพาะ owner (`ErrForbidden`)
  - Day/Plan หาด้วย journeyID ของทริปที่อยู่ใน URL เสมอ ไม่เจอ → `ErrDayNotFound` / `ErrPlanNotFound` (Review Focus 2)
  - `ReturnDate` ก่อน `DepartureDate` → `ValidationError`; title/destination ว่าง → `ValidationError`; `cover_url` ผิดกฎ → `ValidationError`; plan description ว่าง → `ValidationError`
  - ย้าย plan ไปวันที่ไม่ได้อยู่ในทริปนี้ → `ErrDayNotFound`

- [ ] **Step 1: Write failing tests**
  - `TestPatchJourney`: editor แก้ title ได้, field อื่นไม่เปลี่ยน; editor แก้ `EditorsCanInvite` → `ErrForbidden`; traveler → `ErrForbidden`; return ก่อน departure → `ValidationError`; `cover_url "http://x"` → `ValidationError`
  - `TestDayCRUD`: เพิ่ม/แก้/ลบ day; ลบ day แล้ว plan ในวันนั้นหาย
  - `TestPlanCRUDAndPosition`: เพิ่ม 3 plan ได้ position 0,1,2; ลบตัวกลางแล้วเพิ่มใหม่ → position 3
  - `TestMovePlan`: ย้าย plan ไปอีกวัน → `day_id` ใหม่, position = ท้ายวันใหม่
  - `TestReorderPlans`: ส่งลำดับใหม่ → `GetBySlug` ได้ตามลำดับนั้น; ส่งขาด 1 ตัว / มี id แปลก → `ValidationError`
  - `TestCrossTripIDs` (Review Focus 2): ทริป A กับ B ของ alice; `UpdatePlan(slug A, planId ของ B)` → `ErrPlanNotFound`; `DeleteDay(slug A, dayId ของ B)` → `ErrDayNotFound`; ย้าย plan ของ A ไป day ของ B → `ErrDayNotFound`; ข้อมูลของ B ไม่เปลี่ยน
  - `TestEditorFullUpdate`: editor `JourneyService.Update` ได้; editor `Delete` → `ErrForbidden`
- [ ] **Step 2:** Run → FAIL
- [ ] **Step 3:** Implement
- [ ] **Step 4:** Run `go test ./...` → PASS
- [ ] **Step 5: Commit** `feat: add granular itinerary editing`

---

### Task 6: HTTP endpoints + README

**Files:**
- Create: `internal/handlers/member_handler.go`, `internal/handlers/itinerary_handler.go`
- Modify: `internal/handlers/errors.go`, `internal/handlers/journey_handler.go`, `internal/routes/routes.go`, `internal/app/app.go`, `README.md`
- Test: `internal/app/member_http_test.go`, `internal/app/itinerary_http_test.go`

**Interfaces:**
- Consumes: `MemberService`, `ItineraryService`, `AccessService` (Task 3–5)
- Error mapping เพิ่ม: `ErrMemberNotFound`, `ErrInviteNotFound`, `ErrDayNotFound`, `ErrPlanNotFound` → 404 (message ของมัน); `ErrAlreadyMember`, `ErrTripFull` → 409
- `GET /journeys/:slug` เพิ่ม field `members` = `PublicMembers` (owner + joined)
- Endpoints (prefix `/api/v1`, ✅ = `RequireAuth` ต่อ route):

  | Method + path | Auth | Body | สำเร็จ |
  |---|---|---|---|
  | `PATCH /journeys/:slug` | ✅ | `JourneyPatch` | 200 journey |
  | `POST /journeys/:slug/days` | ✅ | `DayInput` | 201 day |
  | `PATCH /journeys/:slug/days/:dayId` | ✅ | `DayInput` | 200 day |
  | `DELETE /journeys/:slug/days/:dayId` | ✅ | – | 200 `{}` |
  | `POST /journeys/:slug/days/:dayId/plans` | ✅ | `PlanInput` | 201 plan |
  | `PUT /journeys/:slug/days/:dayId/plans/order` | ✅ | `{"plan_ids": [...]}` | 200 `{}` |
  | `PATCH /journeys/:slug/plans/:planId` | ✅ | `PlanInput` | 200 plan |
  | `DELETE /journeys/:slug/plans/:planId` | ✅ | – | 200 `{}` |
  | `GET /journeys/:slug/members` | ✅ | – | 200 `[]MemberView` (สมาชิกเท่านั้น) |
  | `POST /journeys/:slug/members` | ✅ | `{"username","role"}` | 201 member |
  | `PATCH /journeys/:slug/members/:username` | ✅ | `{"role"}` | 200 `{}` |
  | `DELETE /journeys/:slug/members/:username` | ✅ | – | 200 `{}` |
  | `POST /journeys/:slug/leave` | ✅ | – | 200 `{}` |
  | `GET /journeys/:slug/invite-links` | ✅ | – | 200 `[]InviteLink` |
  | `POST /journeys/:slug/invite-links` | ✅ | `{"role"}` | 201 `{"url","link"}` |
  | `DELETE /journeys/:slug/invite-links/:id` | ✅ | – | 200 `{}` |
  | `POST /invites/join` | ✅ | `{"token"}` | 200 `{"slug"}` |
  | `GET /me/invites` | ✅ | – | 200 `[]InviteView` |
  | `POST /me/invites/:id/accept` | ✅ | – | 200 `{}` |
  | `POST /me/invites/:id/decline` | ✅ | – | 200 `{}` |
  | `GET /me/journeys` | ✅ | – | 200 รายการทริปที่เป็น owner หรือ joined: `{slug,title,destination,departure_date,return_date,cover_url,role}` เรียง departure_date ใหม่สุดก่อน |

  token ลิงก์เชิญส่งใน body (`POST /invites/join`) ไม่ใส่ใน path เพื่อไม่ให้ไปโผล่ใน access log

- [ ] **Step 1: Write failing tests** (ผ่าน `app.New` + helper login เดิมใน `auth_http_test.go`)
  - `TestHTTP_InviteFlow`: alice สร้างทริป → เชิญ `bob` เป็น editor (201) → `GET /journeys/:slug` ยังไม่มี bob ใน `members` (Review Focus 3) → bob `GET /me/invites` เห็น 1 → accept → `members` มี bob role editor → bob `POST /journeys/:slug/days` ได้ 201
  - `TestHTTP_InvitedCannotEdit` (Review Focus 3): bob ถูกเชิญแต่ยังไม่รับ → `POST /days` → 403
  - `TestHTTP_InviteLinkFlow`: alice สร้างลิงก์ traveler → carol `POST /invites/join` → 200 slug → carol อยู่ใน `members` role traveler → carol `PATCH /plans/:id` → 403
  - `TestHTTP_PlanEditing`: เพิ่ม plan, ย้ายวัน, reorder, ลบ → `GET` สะท้อนครบ
  - `TestHTTP_CrossTripPlan` (Review Focus 2): `PATCH /journeys/A/plans/<planId ของ B>` → 404
  - `TestHTTP_MyJourneys`: alice มี 1 ทริปของตัวเอง + joined ทริปของ bob 1 + ถูกเชิญอีก 1 (ยังไม่รับ) → ได้ 2 รายการ role `owner` กับ `traveler`
  - `TestHTTP_MembersEndpointNeedsMembership`: คนนอก `GET /journeys/:slug/members` → 403
  - ทุก endpoint ในตาราง: ไม่มี token → 401 (table test)
- [ ] **Step 2:** Run → FAIL
- [ ] **Step 3:** Implement handlers, routes, app wiring; README เพิ่มหัวข้อ Members และ Itinerary editing (ตารางด้านบน + กฎ role)
- [ ] **Step 4:** Run `go test ./...`, `go vet ./...`, `go build ./...` → PASS
- [ ] **Step 5: Commit** `feat: add member and itinerary endpoints`

---

### Task 7: คำสั่งยกทริปเก่าให้เจ้าของ

ทริปบน production (Railway) ทุกทริปสร้างก่อนมีระบบ user เลยไม่มีเจ้าของและแก้ไม่ได้ ต้องมีวิธียกให้บัญชีของเจ้าของเว็บหลัง login ครั้งแรก

**Files:**
- Create: `internal/admin/legacy.go`
- Modify: `main.go`, `README.md`
- Test: `internal/admin/legacy_test.go`

**Interfaces:**
- Produces: `admin.AssignLegacyOwner(ctx context.Context, db *gorm.DB, username string) (int64, error)` — หา user ด้วย username (ตัวเล็ก) ไม่เจอ → error `"user not found"`; `UPDATE journeys SET owner_id = <id> WHERE (owner_id IS NULL OR owner_id = '') AND deleted_at IS NULL`; คืนจำนวนแถว
- `main.go`: ถ้า `len(os.Args) > 1 && os.Args[1] == "assign-legacy-owner"` → ต้องมี username ใน `os.Args[2]` → ต่อ DB, migrate, เรียก `AssignLegacyOwner`, log จำนวน, `os.Exit(0)`; ไม่ต้องเปิด server และไม่ต้องตรวจ SMTP / APP_WEB_URL
- README: วิธีใช้ในเครื่อง (`go run . assign-legacy-owner <username>`) และบน Railway (`railway run ./journey-service assign-legacy-owner <username>` หรือรันใน shell ของ service)

- [ ] **Step 1: Write failing tests**: ทริปเก่า 2 อัน (owner `""` 1, NULL 1) + ทริปของ bob 1 + ทริปเก่าที่ถูก soft delete 1 → `AssignLegacyOwner("Alice")` คืน 2, ทริปของ bob ยังเป็นของ bob; เรียกซ้ำคืน 0; username ไม่มี → error
- [ ] **Step 2:** FAIL · **Step 3:** Implement · **Step 4:** `go test ./...`, `go build ./...` → PASS
- [ ] **Step 5: Commit** `feat: add assign-legacy-owner command`

---

## สิ่งที่แผนนี้ตัดสินใจเอง (ไม่ได้มาจาก spec)

- owner ไม่อยู่ในตาราง `journey_members` ใช้ `journeys.owner_id` ตามเดิม ไม่ต้อง backfill ข้อมูลทริปที่มีอยู่
- ปฏิเสธ / ออก / ถูกเอาออก = ลบแถวทิ้ง ไม่เก็บประวัติ (YAGNI)
- กดลิงก์เชิญ = ยินยอมแล้ว เข้าเป็น joined ทันที ส่วนเชิญด้วย username ต้องกดรับ
- editor ที่ได้สิทธิ์เชิญ เชิญได้แค่ traveler; เปลี่ยน role / เอาคนออก / ยกเลิกลิงก์ทำได้แค่ owner
- ลำดับ plan ใช้ `position` ก่อน แล้วค่อย `time`; plan เก่าที่ position เป็น 0 หมด ยังเรียงตามเวลาเหมือนเดิม
- full update (`PUT`) ยังอยู่เพื่อไม่ให้ client เดิมพัง แต่ล้าง ID ของ day/plan ทุกครั้ง
- ยังไม่ทำ: ขอจอยทริป (P1), แจ้งเตือน (P1), โอนความเป็นเจ้าของ, visibility (แผน 3)
