# Privacy, Trip Status & Profile Implementation Plan (MVP แผน 3 จาก 6)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ทุกที่ที่ API ส่งข้อมูลทริปออกไป ต้องผ่านกฎ privacy เดียวกัน (visibility 4 ระดับ + ซ่อนรายละเอียดจากคนนอกระหว่างทริปยังไม่จบ + ซ่อนที่พัก) พร้อมสถานะทริป, สถานะ "กำลังเที่ยว", follow และหน้า profile เต็ม

**Architecture:** เพิ่ม `PrivacyService` เป็นจุดเดียวที่ตัดสินว่า "ใครเห็นอะไร" คืน `JourneyView` ที่ตัดข้อมูลแล้ว handler ทุกตัวที่ส่งทริปออกไป (`GET /journeys`, `GET /journeys/:slug`, `/me/journeys`, profile) ต้องผ่านมัน สถานะทริปเป็น pure function ใน `internal/tripstatus` follow ย้ายมาจากแผน 5 เพราะ visibility "Followers" ต้องใช้

**Tech Stack:** เหมือนแผน 1–2

**Spec:** `docs/business-requirements/2026-11-08-social-trip-sharing.md` ข้อ 6.1, 6.2 (ข้อ 1–3), 7.1, 7.2 (หน้า profile), 7.5, 7.7 (เฉพาะ follow)

**Base:** branch `feature/privacy-status` แตกจาก `feature/trip-members`

## Global Constraints

- Visibility: `public` | `followers` | `link` | `private` ค่าเริ่มต้น **`followers`** (ยืนยันโดยผู้ใช้ 2026-11-08); เปลี่ยนได้เฉพาะ owner
- ทริปเก่าที่ไม่มีเจ้าของ (`owner_id` ว่าง) ถือเป็น `public` เสมอ เพราะก่อนหน้านี้ทุกทริปเปิดสาธารณะ
- ระดับความสัมพันธ์ของคนดู (viewer): `member` (owner/editor/traveler ที่ joined) > `follower` (follow owner) > `stranger` (รวมคนไม่ login)
- เห็นทริปได้ไหม: public → ทุกคน; followers → member + follower; link → ทุกคนที่มี slug แต่ไม่โผล่ใน list ใด ๆ ยกเว้นของ member; private → member เท่านั้น; เห็นไม่ได้ = ตอบ **404** (ไม่บอกว่ามีทริปอยู่)
- **Stranger ระหว่างทริปยังไม่ Completed** เห็นแค่: title, country, destination, เดือนเริ่ม–เดือนจบ (`YYYY-MM`), cover_url, status (`planning`/`upcoming`/`completed` — ห้ามบอก `ongoing` ให้ใช้ `upcoming`), owner (username/display_name/avatar) ไม่เห็น: วันที่แน่นอน, days, plans, members คนอื่น
- Stranger หลังทริป Completed: เห็นเต็มเหมือน follower
- Plan ที่ `hidden_from_public = true` เห็นได้เฉพาะ member
- Plan ใหม่: `category` = `"stay"` หรือ `emoji == "🏨"` → `hidden_from_public` ตั้งเป็น true อัตโนมัติ (ถ้า client ไม่ได้ส่งค่ามาเอง)
- Trip status คิดใน timezone ของทริป (`timezone`, IANA name, ค่าเริ่มต้น `"Asia/Bangkok"`) นับเป็นวันตามปฏิทิน
- "กำลังเที่ยว": member เห็นเต็ม + plan ของวันนี้; follower เห็นเมือง + วันที่ X จาก Y; stranger ไม่เห็น; owner ตั้ง `hide_live_status` ได้ → follower ก็ไม่เห็น
- Follow ได้ทันที ไม่ต้องอนุมัติ, follow ตัวเองไม่ได้
- Field ใหม่ของ Journey (`visibility`, `is_draft`, `timezone`, `hide_live_status`) แก้ได้ทาง `PATCH /journeys/:slug` เท่านั้น; full PUT (`/update`) อัปเดตแค่ allowlist เดิม (title, destination, country, departure_date, return_date) ห้ามเพิ่ม field เหล่านี้เข้า allowlist; ตอน create ตั้งค่าได้
- Client จริงคือ `trip-planner` (Next.js static export) ที่เรียก `GET /journeys` และ `GET /journeys/:slug` ตอน build แบบไม่มี token → หลังแผนนี้ build จะเห็นเฉพาะทริป public/legacy ซึ่งถูกต้องตาม privacy; หน้าเว็บจะย้ายไปโหลดฝั่ง browser ในแผน 6

## Review Focus

1. **ทุก endpoint ที่ส่งทริปออกไปต้องผ่าน PrivacyService** ถ้าหลุดสักที่ (เช่น `/me/journeys` ส่งทริปที่ไม่ได้เป็นสมาชิก, list ทริปตามประเทศ, members list) ข้อมูลจะรั่ว → test ใน Task 5 วนทุก endpoint ด้วย stranger
2. **ทริปที่กำลังเที่ยวอยู่ต้องไม่บอก stranger ว่ากำลังเที่ยว** ทั้งทาง status, วันที่, หรือ profile section "ตอนนี้" → test ใน Task 4 และ 6
3. **ขอบวันในต่าง timezone** ทริปญี่ปุ่น (`Asia/Tokyo`) ที่ return_date คือวันนี้ตามเวลาโตเกียว แต่ UTC ยังเป็นเมื่อวาน → ยังเป็น ongoing → test ใน Task 2
4. **เห็นไม่ได้ต้องเป็น 404 ไม่ใช่ 403** ไม่งั้นคนเดา slug รู้ว่ามีทริปซ่อนอยู่ → test ใน Task 5
5. **ที่พักที่ซ่อน** ต้องไม่หลุดผ่าน full trip ของ follower หรือ stranger หลังทริปจบ → test ใน Task 4

---

## File Structure

| ไฟล์ | หน้าที่ |
|---|---|
| `internal/tripstatus/status.go` | `Compute(j, now) Status` pure function |
| `internal/models/follow.go` | `Follow` |
| `internal/repository/follow_repository.go`, `follow_gorm.go` | follow |
| `internal/services/follow_service.go` | follow/unfollow/counts |
| `internal/services/privacy_service.go` | relation ของ viewer + `View(j, viewer)` → `JourneyView` |
| `internal/services/profile_service.go` | หน้า profile: now / upcoming / calendar / past / stats |
| `internal/handlers/follow_handler.go`, `profile_handler.go` | HTTP |
| แก้ `internal/models/journey.go`, `internal/services/itinerary_service.go`, `internal/services/journey_service.go`, `internal/handlers/journey_handler.go`, `internal/handlers/itinerary_handler.go`, `internal/handlers/member_handler.go`, `internal/routes/routes.go`, `internal/app/app.go`, `database/postgres.go`, `README.md` | |

---

### Task 1: Model fields + Follow repository

**Files:**
- Modify: `internal/models/journey.go`, `database/postgres.go`
- Create: `internal/models/follow.go`, `internal/repository/follow_repository.go`, `internal/repository/follow_gorm.go`
- Test: `internal/repository/follow_gorm_test.go`

**Interfaces:**
- Produces:
  - `Journey.Visibility string` — `gorm:"type:varchar(10);not null;default:'followers'" json:"visibility"`
  - `Journey.IsDraft bool` — `gorm:"not null;default:false" json:"is_draft"`
  - `Journey.Timezone string` — `gorm:"type:varchar(64);not null;default:'Asia/Bangkok'" json:"timezone"`
  - `Journey.HideLiveStatus bool` — `gorm:"not null;default:false" json:"hide_live_status"`
  - `ActivityPlan.Category string` — `gorm:"type:varchar(20)" json:"category"` (ค่าที่รู้จัก: `stay`, `food`, `sight`, `transport`, `other`; ว่างได้)
  - `ActivityPlan.HiddenFromPublic bool` — `gorm:"not null;default:false" json:"hidden_from_public"`
  - constants `VisibilityPublic`, `VisibilityFollowers`, `VisibilityLink`, `VisibilityPrivate`
  - `Follow{FollowerID, FolloweeID string (composite primary key, varchar(10)); CreatedAt}`
  - `FollowRepository`: `Follow(ctx, followerID, followeeID) error` (ซ้ำ = ไม่ error), `Unfollow(ctx, followerID, followeeID) error`, `IsFollowing(ctx, followerID, followeeID) (bool, error)`, `CountFollowers(ctx, userID) (int64, error)`, `CountFollowing(ctx, userID) (int64, error)`, `ListFolloweeIDs(ctx, followerID) ([]string, error)`; `NewFollowRepository(db)`

- [ ] **Step 1: Write failing tests**: follow ซ้ำไม่ error และนับ 1; unfollow แล้ว `IsFollowing` false; counts ถูก; `ListFolloweeIDs` ได้ครบ
- [ ] **Step 2:** Run → FAIL · **Step 3:** Implement · **Step 4:** `go test ./...` → PASS
- [ ] **Step 5: Commit** `feat: add visibility fields and follows`

---

### Task 2: Trip status (pure)

**Files:**
- Create: `internal/tripstatus/status.go`
- Test: `internal/tripstatus/status_test.go`

**Interfaces:**
- Produces:
  ```go
  type Status struct {
      State      string // "planning" | "upcoming" | "ongoing" | "completed"
      DaysUntil  int    // upcoming: วันปฏิทินจากวันนี้ถึงวันเดินทาง (≥1)
      DayNumber  int    // ongoing: วันที่ X (เริ่ม 1)
      TotalDays  int    // ongoing/upcoming/completed: จำนวนวันของทริป (return - departure + 1)
  }
  func Compute(departure, ret time.Time, tz string, isDraft bool, now time.Time) Status
  ```
- `departure`/`return` เป็นวันที่ (ใช้ปี-เดือน-วัน ตามที่เก็บ, ไม่สน time-of-day); "วันนี้" = `now.In(loc)` เป็นปี-เดือน-วัน; tz โหลดไม่ได้ → ใช้ `Asia/Bangkok`
- isDraft → planning เสมอ

- [ ] **Step 1: Write failing tests** (table): draft → planning; วันนี้ก่อนออก 3 วัน → upcoming DaysUntil 3; วันออก → ongoing day 1/5; วันกลับ → ongoing day 5/5; วันหลังกลับ → completed; **Review Focus 3**: ทริป `Asia/Tokyo` return 2026-12-16, now = `2026-12-16T00:30+09:00` (ยังเป็น 15 ธ.ค. ใน UTC) → ongoing; now = `2026-12-17T00:10+09:00` → completed; tz `"Mars/Base"` → fallback ไม่ panic
- [ ] **Step 2:** FAIL · **Step 3:** Implement · **Step 4:** PASS
- [ ] **Step 5: Commit** `feat: add trip status computation`

---

### Task 3: ตั้งค่า privacy ผ่าน API ที่มีอยู่

**Files:**
- Modify: `internal/services/itinerary_service.go` (JourneyPatch + PlanInput), `internal/services/journey_service.go` (Create/Update)
- Test: `internal/services/itinerary_service_test.go`, `internal/services/journey_service_privacy_test.go`

**Interfaces:**
- `JourneyPatch` เพิ่ม `Visibility *string`, `IsDraft *bool`, `Timezone *string`, `HideLiveStatus *bool` — `Visibility` และ `HideLiveStatus` เปลี่ยนได้เฉพาะ owner (`ErrForbidden`); editor เปลี่ยน `IsDraft`, `Timezone` ได้
- `PlanInput` เพิ่ม `Category *string`, `HiddenFromPublic *bool`
- `services.autoHide(category, emoji string) bool` → `category == "stay" || emoji == "🏨"`
- กฎ: visibility นอก 4 ค่า → `ValidationError`; timezone ที่ `time.LoadLocation` ไม่ได้ → `ValidationError`; category นอกรายการ → `ValidationError`
- `AddPlan` / Create / full Update: ถ้า `HiddenFromPublic` ไม่ได้ส่งมา (nil ใน PlanInput / false ใน full body) ใช้ `autoHide`; `UpdatePlan` ที่เปลี่ยน category/emoji แต่ไม่ส่ง HiddenFromPublic → คำนวณใหม่
- full PUT (`/update`) ไม่แตะ `Visibility`, `IsDraft`, `Timezone`, `HideLiveStatus` (allowlist ของแผน 2) — test: owner PUT body ที่มี `"visibility":"public"` → visibility ไม่เปลี่ยน
- Create: `Visibility` ว่าง → `followers`; ค่าไม่ถูก → `ValidationError`; `Timezone` ว่าง → `Asia/Bangkok`

- [ ] **Step 1: Write failing tests**: editor เปลี่ยน visibility → `ErrForbidden`; owner เปลี่ยนเป็น `link` ได้; `"secret"` → `ValidationError`; timezone `"Asia/Tokyo"` ได้, `"Nope/Zone"` → `ValidationError`; plan emoji 🏨 → hidden true; category `stay` → hidden true; ส่ง `hidden_from_public:false` มาเองกับ 🏨 → false; สร้างทริปไม่ส่ง visibility → `followers`
- [ ] **Step 2:** FAIL · **Step 3:** Implement · **Step 4:** PASS
- [ ] **Step 5: Commit** `feat: add privacy settings to trips and plans`

---

### Task 4: PrivacyService

**Files:**
- Create: `internal/services/privacy_service.go`
- Test: `internal/services/privacy_service_test.go`

**Interfaces:**
- Consumes: `AccessService` (แผน 2), `FollowRepository`, `MemberRepository.ListByJourney`, `UserRepository.GetByID`, `tripstatus.Compute`
- Produces:
  ```go
  const (RelMember = "member"; RelFollower = "follower"; RelStranger = "stranger")
  type JourneyView struct {
      Slug, Title, Destination, Country, CoverURL, Visibility string
      Owner        models.PublicProfile
      Status       tripstatus.Status   // stranger + ไม่ completed: State "ongoing" → "upcoming", DayNumber=0, DaysUntil=0
      MonthStart, MonthEnd string        // "YYYY-MM" ทุกคนเห็น
      DepartureDate, ReturnDate *time.Time          // nil เมื่อ redacted
      Days    []models.ItineraryDay                   // nil เมื่อ redacted; plan ที่ hidden ถูกตัดออกถ้าไม่ใช่ member
      Members []models.MemberView                     // owner + joined; nil เมื่อ redacted
      Redacted bool
      Role     string   // role ของ viewer ถ้าเป็น member
      Live     *LiveView // เฉพาะ ongoing และ viewer มีสิทธิ์เห็น (member หรือ follower ที่ไม่ได้ซ่อน)
  }
  type LiveView struct { City string; DayNumber, TotalDays int; TodayPlans []models.ActivityPlan /* member เท่านั้น */ }
  func NewPrivacyService(d PrivacyDeps) *PrivacyService   // PrivacyDeps{Access, Follows, Members, Users, Now}
  func (s *PrivacyService) Relation(ctx, viewerID string, j *models.Journey, role string) (string, error)
  func (s *PrivacyService) CanSee(rel string, j *models.Journey) bool
  func (s *PrivacyService) View(ctx, viewerID, slug string) (*JourneyView, error)    // เห็นไม่ได้ → ErrNotFound
  func (s *PrivacyService) ViewOf(ctx, viewerID string, j *models.Journey) (*JourneyView, bool, error) // bool=เห็นได้ ใช้ใน list
  ```
  json tags เป็น snake_case ทั้งหมด (`month_start`, `departure_date`, `live`, …)
- `LiveView.City` = `Destination`; `TodayPlans` = plan ของ day ที่ `DateISO` ตรงกับวันนี้ใน tz ของทริป (ตัด hidden ออกถ้าไม่ใช่ member — แต่ TodayPlans มีเฉพาะ member อยู่แล้ว)

- [ ] **Step 1: Write failing tests** (table ครอบทุกคู่ visibility × relation × status)
  - public + stranger + upcoming → เห็น, `Redacted` true, ไม่มี DepartureDate/Days/Members, มี MonthStart
  - public + stranger + ongoing (Review Focus 2) → `Status.State == "upcoming"`, `Live == nil`, `Redacted`
  - public + stranger + completed → เต็ม แต่ plan hidden ถูกตัด (Review Focus 5)
  - followers + stranger → `ErrNotFound`; followers + follower + ongoing → เต็ม, `Live` มี City/DayNumber แต่ไม่มี TodayPlans, plan hidden ถูกตัด
  - followers + follower + ongoing + `HideLiveStatus` → `Live == nil`
  - link + stranger → เห็นแบบ redacted (ยังไม่จบ); private + follower → `ErrNotFound`; private + member → เต็ม
  - member (traveler) ongoing → `Live.TodayPlans` มี plan ของวันนี้ รวม hidden
  - invited (ยังไม่รับ) ถือเป็น stranger/follower ตามจริง ไม่ใช่ member
  - legacy (owner ว่าง) → ถือ public
- [ ] **Step 2:** FAIL · **Step 3:** Implement · **Step 4:** PASS
- [ ] **Step 5: Commit** `feat: add privacy service`

---

### Task 5: ต่อ PrivacyService เข้าทุก endpoint ที่ส่งทริปออก

**Files:**
- Modify: `internal/handlers/journey_handler.go`, `internal/handlers/member_handler.go`, `internal/services/journey_service.go`, `internal/routes/routes.go`, `internal/app/app.go`
- Test: `internal/app/privacy_http_test.go`

**Interfaces:**
- `GET /journeys/:slug` → `PrivacyService.View` (viewer = user ถ้ามี token ที่ถูกต้อง, ไม่งั้น stranger) → ต้องมี middleware `OptionalAuth(svc)` ที่ตั้ง `user_id` ถ้า token ถูก และ **ไม่** 401 ถ้าไม่มี/ผิด
- `GET /journeys` (list ตามประเทศ) → แสดงเฉพาะทริปที่ `CanSee` และ visibility ≠ `link` (ยกเว้นทริปที่ viewer เป็น member) และใช้ข้อมูลจาก `JourneyView`
- `GET /me/journeys` → เฉพาะทริปที่เป็น owner หรือ joined (เดิม) + เพิ่ม `status`
- `GET /journeys/:slug/members` (แผน 2) ยังเป็น member-only; members ใน journey detail มาจาก `JourneyView.Members`
- Write endpoints (PATCH, days, plans, members) ยังใช้ AccessService ตามเดิม แต่ถ้า viewer เห็นทริปไม่ได้ (ไม่ใช่ member) ให้ตอบ **404** แทน 403 — ใช้ `respondError` กับ `ErrNotFound` เมื่อ Access.Role == "" และทริปไม่ public/link

- [ ] **Step 1: Write failing tests**
  - `TestHTTP_StrangerSweep` (Review Focus 1): ทริป followers ของ alice ที่กำลังเที่ยว; ยิงทุก GET (`/journeys`, `/journeys/:slug`, `/journeys/:slug/members`, `/users/alice/profile` [Task 6 จะเพิ่ม — ใส่ใน Task 6]) แบบไม่มี token และด้วย token ของ stranger → ไม่มี response ไหนมี slug/วันที่/`"ongoing"` ของทริปนั้น
  - `TestHTTP_HiddenIs404` (Review Focus 4): private trip → `GET` โดย stranger = 404 body `"journey not found"`; `PATCH` โดย stranger = 404
  - `TestHTTP_FollowerSeesFullTrip`: bob follow alice → `GET` ทริป followers ได้เต็ม
  - `TestHTTP_LinkNotListed`: ทริป `link` ไม่อยู่ใน `GET /journeys` แต่ `GET /journeys/:slug` ได้ (redacted ถ้ายังไม่จบ)
  - `TestHTTP_OptionalAuthBadToken`: token มั่วบน `GET /journeys/:slug` ของทริป public → 200 แบบ stranger (ไม่ใช่ 401)
- [ ] **Step 2:** FAIL · **Step 3:** Implement · **Step 4:** PASS
- [ ] **Step 5: Commit** `feat: apply privacy rules to trip endpoints`

---

### Task 6: Follow + Profile endpoints

**Files:**
- Create: `internal/services/follow_service.go`, `internal/services/profile_service.go`, `internal/handlers/follow_handler.go`, `internal/handlers/profile_handler.go`
- Modify: `internal/routes/routes.go`, `internal/app/app.go`, `internal/handlers/errors.go`, `README.md`
- Test: `internal/app/profile_http_test.go`, `internal/services/profile_service_test.go`

**Interfaces:**
- `FollowService`: `Follow(ctx, followerID, username) error` (ตัวเอง → `ValidationError("cannot follow yourself")`, ไม่มี user → `ErrUserNotFound`), `Unfollow`, `Counts(ctx, username) (followers, following int64, err error)`
- `ProfileService.Get(ctx, viewerID, username) (*ProfileView, error)`:
  ```go
  type ProfileView struct {
      User        models.PublicProfile
      Followers, Following int64
      IsFollowing bool                 // viewer follow คนนี้อยู่ไหม
      Now         *JourneyView         // ทริป ongoing ที่ viewer เห็น Live ได้ (เอาอันแรก); nil ถ้าไม่มีหรือเห็นไม่ได้
      Upcoming    []JourneyView        // เรียงวันออกใกล้สุดก่อน
      Calendar    []CalendarItem       // ทุกทริปที่ viewer เห็นได้: {slug,title,destination,month_start,month_end,departure_date?,return_date?,companions []PublicProfile?} — วันที่และ companions ตามกฎ redaction เดียวกับ JourneyView
      Past        []JourneyView        // completed ล่าสุดก่อน
      Stats       Stats                // {countries, cities, trips} นับจาก Past ที่ viewer เห็นได้เท่านั้น
  }
  ```
  ทริปที่นับ = owner หรือ joined ของเจ้าของ profile; เห็นได้หรือไม่ใช้ `PrivacyService.ViewOf`
- Endpoints (`/api/v1`): `POST /users/:username/follow` ✅, `DELETE /users/:username/follow` ✅, `GET /users/:username/profile` (OptionalAuth) → `ProfileView`
- `ErrUserNotFound` → 404 (มีแล้ว)

- [ ] **Step 1: Write failing tests**
  - `TestProfile_NowHiddenFromStranger` (Review Focus 2): alice กำลังเที่ยว (ทริป public) → stranger: `Now == nil`, ทริปนั้นอยู่ใน `Upcoming`? **ไม่** — อยู่ใน Calendar แบบ redacted เท่านั้น ไม่มีวันที่; follower: `Now` มี Live City/DayNumber
  - `TestProfile_Stats`: alice มีทริป completed public ญี่ปุ่น 2 เมือง + ไทย 1 + ทริป private completed เกาหลี → stranger เห็น countries 2 cities 3 trips 3; member ทริปเกาหลีเห็น countries 3
  - `TestProfile_IncludesJoinedTrips`: bob joined ทริปของ alice (public completed) → profile ของ bob มีทริปนั้นใน Past; invited ยังไม่รับ → ไม่มี
  - `TestHTTP_Follow`: follow → counts 1, `is_following` true; follow ตัวเอง → 400; unfollow → 0
  - `TestHTTP_StrangerSweep` เพิ่ม `/users/alice/profile`
- [ ] **Step 2:** FAIL · **Step 3:** Implement; README: หัวข้อ Privacy (ตาราง visibility + redaction), Profile, Follow · **Step 4:** `go test ./...`, `go vet ./...`, `go build ./...` → PASS
- [ ] **Step 5: Commit** `feat: add follow and profile endpoints`

---

## สิ่งที่แผนนี้ตัดสินใจเอง (ไม่ได้มาจาก spec)

- ย้าย follow มาจากแผน 5 เพราะ visibility "Followers" ใช้ไม่ได้ถ้าไม่มี follow (feed ยังอยู่แผน 5)
- เห็นไม่ได้ตอบ 404 ไม่ใช่ 403 เพื่อไม่ให้รู้ว่ามีทริปอยู่
- stranger ระหว่างทริปกำลังเที่ยวเห็น status เป็น "upcoming" แทน "ongoing" เพื่อไม่บอกว่ากำลังไม่อยู่บ้าน
- "คนนอก" ของที่พักที่ซ่อน = คนที่ไม่ใช่ member (follower ก็ไม่เห็น)
- ทริปเก่าที่ไม่มีเจ้าของ ถือเป็น public
- timezone ค่าเริ่มต้น `Asia/Bangkok` (ผู้ใช้หลักเป็นคนไทย) ทริปต่างประเทศควรตั้งเอง — หน้าเว็บ (แผน 6) จะเดา tz จากประเทศให้
- stats นับเฉพาะทริปที่คนดูเห็นได้ ไม่งั้นจำนวนประเทศจะบอกใบ้ทริป private
