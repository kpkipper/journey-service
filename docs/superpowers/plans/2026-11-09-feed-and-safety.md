# Feed, Block & Report Implementation Plan (MVP แผน 5 จาก 6)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** มี feed เรียงตามเวลาที่บอกว่าเพื่อนจะไปไหน เริ่มทริป แปะรูป และเที่ยวจบ พร้อมแถว "ตอนนี้ใครเที่ยวอยู่" และผู้ใช้ block / report คนอื่นได้

**Architecture:** Block เก็บเป็นตาราง `blocks` และบังคับที่ `AccessService` จุดเดียว (ทริปของคนที่ block กันเห็นไม่ได้ = 404 ยกเว้นเป็น member ทริปเดียวกัน) แล้วเพิ่มด่านที่ profile, follow, invite, join link และ moments Feed ไม่มีตาราง event: คำนวณตอนอ่านจากทริปของคนที่เรา follow + ทริปที่เราเป็น member โดยทุกทริปต้องผ่าน `PrivacyService.ViewOf` และต้องไม่ redacted Report เก็บลงตาราง `reports` และดู/ปิดผ่าน CLI

**Tech Stack:** เหมือนแผน 1–4

**Spec:** `docs/business-requirements/2026-11-08-social-trip-sharing.md` ข้อ 6.2 (ข้อ 7), 7.7

**Base:** branch `feature/feed-safety` แตกจาก `feature/moments`

## Global Constraints

- **Block** เป็นความสัมพันธ์ทางเดียวในตาราง แต่ผลบังคับทั้งสองทาง ("block กัน" = A block B หรือ B block A)
- Block แล้ว: ลบ follow ทั้งสองทางทันที; คนที่ block กัน follow กันไม่ได้ (ตอบ 404 `user not found` เหมือนไม่มี user); invite กันด้วย username ไม่ได้ (404 `user not found`); join link ของทริปที่ owner block กันกับคนกด → 404 `invite not found`
- Profile: คนที่ถูก block เปิด `GET /users/:username/profile` และ `GET /users/:username` ของคนที่ block เขา → 404; คนที่ block เป็นฝ่ายเปิดเอง → 200 แต่ `is_blocked: true` และ `now: null`, `upcoming/calendar/past: []`, stats เป็น 0 (ให้กด unblock ได้)
- ทริป: คนที่ block กันกับ **owner** และไม่ได้เป็น member ทริปนั้น → เห็นทริปไม่ได้ (404 ทุก endpoint, ไม่โผล่ใน list ใด ๆ); ถ้าเป็น member อยู่แล้วยังใช้ทริปได้ตามปกติ (owner เอาออกเองได้)
- Moments: ไม่แสดง Moment ที่คนอัปโหลดกับคนดู block กัน
- Block ตัวเอง → 400; block ซ้ำ → 204 (idempotent); unblock ที่ไม่ได้ block → 204
- **Report** target: `user` (username), `journey` (slug), `moment` (id); reason: `spam` | `harassment` | `inappropriate` | `privacy` | `other`; note ≤ 500 rune; target ต้องมีอยู่**และ**คน report ต้องเห็นได้ ไม่งั้น 404; report ซ้ำ target เดิมขณะที่อันเดิมยัง `open` → 200 คืนอันเดิม; report ใหม่ → 201; คนหนึ่ง report ได้ ≤ **20 ครั้ง / 24 ชม.** → 429
- CLI: `./server reports` (พิมพ์ report ที่ open ทีละบรรทัด JSON), `./server resolve-report <id>`, `./server delete-moment <id>` (ลบ Moment + ไฟล์ ใช้ storage config เดียวกับ server)
- **Feed** (`GET /feed`): login เท่านั้น; แหล่งทริป = ทริปที่ owner เป็นคนที่เรา follow + ทริปที่เราเป็น member joined (รวมที่เราเป็น owner); ต้อง `ViewOf` ok, ไม่ redacted, `Listable`, ไม่ใช่ draft; ทริปของคนที่ block กันไม่มี
- ชนิด event และเวลา:
  - `trip_planned` — `created_at` ของทริป, actor = owner
  - `trip_started` — 00:00 ของ `departure_date` ใน timezone ของทริป (เมื่อเวลานั้น ≤ now); **ไม่แสดง** ให้คนที่ไม่ใช่ member ถ้าทริปตั้ง `hide_live_status`
  - `trip_completed` — 00:00 ของวันถัดจาก `return_date` ใน timezone ของทริป (เมื่อ ≤ now), actor = owner
  - `moments_added` — Moment ready ที่คนดูเห็นได้ (กฎเดียวกับ `GET /journeys/:slug/moments`) จัดกลุ่มตาม (คนอัปโหลด, day, วันที่ตามปฏิทินของ `created_at` ใน timezone ของทริป); เวลา = `created_at` ล่าสุดในกลุ่ม; actor = คนอัปโหลด; แนบ `count`, `day_id`, `day_title`, `thumbs` (thumb URL ≤ 4 อันแรก)
- เรียงใหม่สุดก่อน, หน้าละ `limit` (ค่าเริ่มต้น 20, สูงสุด 50), cursor `before` เป็นสตริงทึบ (opaque) ที่ server ออกให้ใน `next_before` (`null` เมื่อหมด); cursor ผิดรูป → 400; คำนวณจากทริปผู้สมัครไม่เกิน **300** ทริปที่สร้างล่าสุด
- **แถว "ตอนนี้ใครเที่ยวอยู่"** (`GET /feed/now`): คนที่เรา follow ที่เป็น owner หรือ member joined ของทริปที่ `JourneyView.Live != nil` สำหรับเรา (= ongoing และคนดูมีสิทธิ์เห็น live); ไม่รวมตัวเองและคนที่ block กัน; คนเดียวมีหลายทริปให้เอาทริปที่เริ่มล่าสุด; เรียงตาม departure ใหม่สุดก่อน
- `companions` ใน event ทริป = member joined อื่นนอกจาก actor (owner นับด้วยถ้าไม่ใช่ actor) ไม่เกิน 5 คน ไม่รวมคนที่ block กับคนดู

## Review Focus

1. **Block รั่วผ่าน endpoint ที่ลืม** — `GET /journeys` (list ตามประเทศ), `/me/journeys`, profile ของคนที่สาม (calendar companions), members list, moments, feed, feed/now → test ใน Task 2 วนครบทุก endpoint
2. **Feed บอกคนนอกว่ากำลังเที่ยว** — ทริป public ของคนที่เรา follow ไม่มีปัญหา แต่ทริป `hide_live_status` ต้องไม่มี `trip_started` และไม่อยู่ใน `/feed/now` → test ใน Task 4
3. **Cursor ซ้ำหรือข้าม** — หลาย event เวลาเดียวกันเป๊ะ (ทริปหลายอันเริ่มเที่ยงคืนเดียวกัน) ต้องไม่ซ้ำและไม่หายข้ามหน้า → test ใน Task 4
4. **Report ใช้สืบว่ามีของซ่อนอยู่** — report ทริป private ด้วย slug ที่เดาได้ ต้องได้ 404 เหมือน GET → test ใน Task 3
5. **Moment ที่ซ่อน (ที่พัก) โผล่ใน feed thumbs** → test ใน Task 4

---

## File Structure

| ไฟล์ | หน้าที่ |
|---|---|
| `internal/models/block.go`, `internal/models/report.go` | ตาราง |
| `internal/repository/block_repository.go`, `block_gorm.go`, `report_repository.go`, `report_gorm.go` | DB |
| `internal/services/block_service.go` | block / unblock / list |
| `internal/services/report_service.go` | report + resolve |
| `internal/services/feed_service.go` | feed + now |
| `internal/handlers/block_handler.go`, `report_handler.go`, `feed_handler.go` | HTTP |
| `internal/admin/reports.go` | CLI reports / resolve / delete-moment |
| แก้ `access_service.go`, `privacy_service.go`, `profile_service.go`, `follow_service.go`, `member_service.go`, `moment_service.go`, `user_handler.go`, `routes.go`, `app.go`, `main.go`, `database/postgres.go`, `README.md` | |

---

### Task 1: Block model, repository, service + follow/profile guards

**Files:**
- Create: `internal/models/block.go`, `internal/repository/block_repository.go`, `internal/repository/block_gorm.go`, `internal/services/block_service.go`
- Modify: `database/postgres.go`, `internal/services/follow_service.go`, `internal/services/profile_service.go`, `internal/services/user_service.go` (หรือ handler ของ `GET /users/:username`)
- Test: `internal/repository/block_gorm_test.go`, `internal/services/block_service_test.go`

**Interfaces:**
- Produces:
  - `type Block struct { BlockerID string (varchar(10) pk); BlockedID string (varchar(10) pk, index); CreatedAt time.Time }`
  - `type BlockRepository interface { Block(ctx, blockerID, blockedID string) error /* idempotent */; Unblock(ctx, blockerID, blockedID string) error; Blocked(ctx, a, b string) (bool, error) /* either direction */; ListBlocked(ctx, blockerID string) ([]models.PublicProfile, error) /* newest first */; RelatedIDs(ctx, userID string) (map[string]bool, error) /* ทุกคนที่ block กับ userID ทั้งสองทาง */ }`
  - `FollowRepository` เพิ่ม `ListFollowerIDs(ctx, followeeID string) ([]string, error)` ถ้าต้องใช้ (ไม่บังคับ)
  - `func NewBlockService(users repository.UserRepository, blocks repository.BlockRepository, follows repository.FollowRepository) *BlockService`
  - `func (s *BlockService) Block(ctx, blockerID, username string) error` — ลบ follow ทั้งสองทางใน transaction เดียวกับการ insert block ถ้าทำได้ (ไม่ได้ก็เรียงลำดับ block ก่อนแล้วลบ follow); ตัวเอง → `ValidationError{"cannot block yourself"}`; ไม่มี user → `ErrUserNotFound`
  - `func (s *BlockService) Unblock(ctx, blockerID, username string) error`
  - `func (s *BlockService) List(ctx, blockerID string) ([]models.PublicProfile, error)`
  - `FollowService` รับ `BlockRepository` (เพิ่มผ่าน `WithBlocks(b) *FollowService`): follow คนที่ block กัน → `ErrUserNotFound`
  - `ProfileDeps.Blocks repository.BlockRepository` (nil = ไม่มี block): viewer ถูก block → `ErrUserNotFound`; viewer เป็นฝ่าย block → profile ว่างตาม Global Constraints + `ProfileView.IsBlocked bool \`json:"is_blocked"\``
  - `GET /users/:username` เปลี่ยนเป็น optionalAuth และคืน 404 เมื่อ viewer ถูก block (Task 5 ต่อ route; Task นี้ทำ service function `UserService.PublicProfileFor(ctx, viewerID, username string)` ที่รับ blocks)
- Consumes: repo เดิม

- [ ] **Step 1: Write the failing tests** — repo: block idempotent, `Blocked` สองทาง, `RelatedIDs`; service: block ลบ follow สองทาง, follow หลัง block ทั้งสองฝั่งได้ `ErrUserNotFound`, unblock แล้ว follow ได้ใหม่, block ตัวเองได้ `ValidationError`; profile: ถูก block → `ErrUserNotFound`, ฝ่าย block → `is_blocked: true` + ว่าง, คนที่สามไม่ได้รับผล
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run** `go test ./...` → PASS
- [ ] **Step 5: Commit** `feat(safety): block users`

---

### Task 2: บังคับ block ระดับทริป, invite, join link, moments

**Files:**
- Modify: `internal/services/access_service.go`, `internal/services/privacy_service.go`, `internal/services/member_service.go`, `internal/services/moment_service.go`, `internal/app/app.go` (wiring `WithBlocks`)
- Test: `internal/services/block_enforcement_test.go`, `internal/app/block_http_test.go`

**Interfaces:**
- Consumes: Task 1 `BlockRepository`
- Produces:
  - `func (s *AccessService) WithBlocks(b repository.BlockRepository) *AccessService`
  - `func (s *AccessService) HiddenByBlock(ctx context.Context, viewerID string, a *Access) (bool, error)` — true เมื่อ viewer ไม่ใช่ member, ทริปมี owner, และ viewer กับ owner block กัน
  - `PrivacyService.view` คืน `ok=false` เมื่อ `HiddenByBlock`; `AccessService.Deny` คืน `ErrNotFound` เมื่อ `HiddenByBlock`
  - `MemberService.Invite` → `ErrUserNotFound` เมื่อ actor หรือ owner block กับ target; `JoinByLink` → `ErrInviteNotFound` เมื่อ owner block กับคนกด
  - `MomentService.List` / `ListMine` ตัด Moment ที่คนอัปโหลด block กับ viewer (ListMine ไม่ต้อง เพราะเป็นของตัวเอง)
- [ ] **Step 1: Write the failing tests** — `TestBlock_TripInvisibleEverywhere` (HTTP, owner A block B: B ได้ 404 ที่ `GET /journeys/:slug` ของทริป public ของ A, ไม่เจอใน `GET /journeys`, ไม่เจอใน profile ของคนที่สาม C ที่เป็น member ทริปนั้น (calendar/upcoming/past), `PATCH /journeys/:slug` ได้ 404 ไม่ใช่ 403, `GET /journeys/:slug/moments` 404; ทิศกลับ (B block A) ผลเหมือนกัน); `TestBlock_MemberKeepsAccess` (B เป็น member อยู่แล้วแล้ว A block B → B ยังเปิดทริปได้); `TestBlock_InviteAndJoinLink`; `TestBlock_HidesMomentsOfBlockedUploader` (A, B, C member ทริปเดียวกัน, C block B → C ไม่เห็น moment ของ B แต่ A เห็น)
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run** `go test ./...` → PASS
- [ ] **Step 5: Commit** `feat(safety): enforce blocks on trips, invites and moments`

---

### Task 3: Report + CLI

**Files:**
- Create: `internal/models/report.go`, `internal/repository/report_repository.go`, `internal/repository/report_gorm.go`, `internal/services/report_service.go`, `internal/admin/reports.go`
- Modify: `database/postgres.go`, `main.go`
- Test: `internal/services/report_service_test.go`, `internal/admin/reports_test.go`

**Interfaces:**
- Produces:
  - `type Report struct { ID string (varchar(10) pk nanoid); ReporterID string (index); TargetType string (varchar(10)); TargetID string (varchar(10), index) /* user id / journey id / moment id — ไม่ใช่ username หรือ slug */; Reason string (varchar(20)); Note string (varchar(2000)); Status string (varchar(10), default 'open', index); CreatedAt time.Time; ResolvedAt *time.Time }`
  - `type ReportInput struct { TargetType, Target, Reason, Note string }` (JSON `target_type, target, reason, note`)
  - `func NewReportService(d ReportDeps) *ReportService` (`ReportDeps{ Reports; Users; Privacy; Moments *MomentService; Blocks; Now }`)
  - `func (s *ReportService) Create(ctx, reporterID string, in ReportInput) (r *models.Report, created bool, err error)` — resolve target: user → `GetByUsername` (ไม่เจอ หรือ block กันแบบที่ reporter ถูก block → `ErrUserNotFound`; report ตัวเอง → `ValidationError`); journey → `Privacy.View` (ไม่เห็น → `ErrNotFound`); moment → ต้องอยู่ใน `MomentService.List` ของทริปนั้นสำหรับ reporter (ไม่เห็น → `ErrMomentNotFound`); reason/note validate → `ValidationError`; rate limit → `ErrTooManyRequests`
  - `func (s *ReportService) ListOpen(ctx) ([]models.Report, error)`, `func (s *ReportService) Resolve(ctx, id string) error` (ไม่เจอ → `ErrReportNotFound`)
  - `MomentService.AdminDelete(ctx, momentID string) error` — ลบโดยไม่เช็คสิทธิ์ (ใช้จาก CLI เท่านั้น)
  - `internal/admin`: `func PrintOpenReports(ctx, w io.Writer, s *services.ReportService) error`, `func ResolveReport(...)`, `func DeleteMoment(...)`; `main.go` dispatch `reports`, `resolve-report <id>`, `delete-moment <id>` (delete-moment ต้อง init storage ก่อน; คำสั่งอื่นไม่ต้อง)
- [ ] **Step 1: Write the failing tests** — report user/journey/moment สำเร็จ (201 = created true); ซ้ำขณะ open → created false อันเดิม; หลัง resolve report ใหม่ได้; ทริป private ที่มองไม่เห็น → `ErrNotFound`; moment ของทริปที่ redacted สำหรับ reporter → `ErrMomentNotFound`; reason แปลก, note 501 rune → `ValidationError`; ครั้งที่ 21 ใน 24 ชม. → `ErrTooManyRequests`; admin: `PrintOpenReports` พิมพ์ JSON หนึ่งบรรทัดต่อ report, `ResolveReport` id ไม่มี → error
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run** `go test ./...` → PASS
- [ ] **Step 5: Commit** `feat(safety): report users, trips and moments with admin CLI`

---

### Task 4: Feed + now row

**Files:**
- Create: `internal/services/feed_service.go`
- Modify: repository เท่าที่ต้อง (เช่น `JourneyRepository.ListByOwners(ctx, ownerIDs []string, limit int)`, `MomentRepository.ListReadyByJourneys(ctx, ids []string)`)
- Test: `internal/services/feed_service_test.go`

**Interfaces:**
- Consumes: `PrivacyService.ViewOf/Relation/Listable`, `MomentService` (กฎการเห็น moment ต้องใช้โค้ดเดียวกับ `List` — แยก helper `visibleMoments(ctx, viewerID string, v *JourneyView, ms []models.Moment) []MomentView` ใน moment_service.go แล้วเรียกจากทั้งสองที่), `BlockRepository.RelatedIDs`, `FollowRepository.ListFolloweeIDs`, `MemberRepository.ListJourneyIDsForUser`
- Produces:
  - `type FeedTrip struct { Slug, Title, Destination, Country, CoverURL string; DepartureDate, ReturnDate *time.Time }` (JSON snake_case)
  - `type FeedMoments struct { Count int; DayID, DayTitle string; Thumbs []string }` (JSON `count, day_id, day_title, thumbs`)
  - `type FeedItem struct { ID string; Type string; At time.Time; Actor models.PublicProfile; Trip FeedTrip; Companions []models.PublicProfile; Moments *FeedMoments }` (JSON `id, type, at, actor, trip, companions, moments(omitempty)`; `ID` = deterministic key เช่น `trip_started:<journeyID>`)
  - `type FeedPage struct { Items []FeedItem \`json:"items"\`; NextBefore *string \`json:"next_before"\` }`
  - `type NowTraveler struct { User models.PublicProfile; Trip FeedTrip; DayNumber, TotalDays int }` (JSON `user, trip, day_number, total_days`)
  - `func NewFeedService(d FeedDeps) *FeedService`
  - `func (s *FeedService) Feed(ctx, viewerID, before string, limit int) (*FeedPage, error)` — cursor ผิดรูป → `ValidationError{"invalid cursor"}`; limit ≤0 → 20, >50 → 50
  - `func (s *FeedService) Now(ctx, viewerID string) ([]NowTraveler, error)`
  - Cursor: เรียง (At desc, ID desc) และ cursor = base64url ของ `<At unixnano>|<ID>` เอา item ที่ (At, ID) น้อยกว่า cursor แบบ lexicographic
- [ ] **Step 1: Write the failing tests** (ใช้ `Now` fake ใน deps)
  - `TestFeed_EventsFromFollowedAndMemberTrips`: ทริป upcoming ของคนที่ follow → `trip_planned` มี companions; ทริป ongoing → `trip_planned` + `trip_started`; ทริปจบ → ครบ 3 + `trip_completed`; ทริปของคนที่ไม่ได้ follow และไม่ได้เป็น member → ไม่มี
  - `TestFeed_ExcludesDraftLinkPrivateRedacted`: draft, link (ไม่ใช่ member), private (ไม่ใช่ member) → ไม่มี event
  - `TestFeed_HideLiveStatusDropsStarted`: follower ไม่เห็น `trip_started`, member เห็น
  - `TestFeed_MomentsGrouped`: B แปะ 3 รูป Day 1 วันเดียวกัน + 1 รูป Day 2 → 2 event `count` 3 และ 1, thumbs ≤ 4, เวลา = รูปล่าสุด
  - `TestFeed_HiddenPlanMomentsNotInFollowerFeed`
  - `TestFeed_TimezoneBoundary`: ทริป `Asia/Tokyo` departure 2026-12-12 → `trip_started.at` = 2026-12-11T15:00:00Z
  - `TestFeed_PaginationWithTies`: 5 ทริปเริ่มเวลาเดียวกัน limit 2 → 3 หน้า ไม่ซ้ำ ไม่ขาด, หน้าสุดท้าย `next_before` null; cursor ขยะ → `ValidationError`
  - `TestFeed_BlockedUsersExcluded`
  - `TestNow_ListsFollowedTravelers`: follow A ที่ ongoing → มี (day_number ถูก); A ตั้ง hide_live → ไม่มี; ตัวเองไม่อยู่; คนที่ไม่ได้ follow ไม่อยู่; คนที่ block กันไม่อยู่
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run** `go test ./...` → PASS
- [ ] **Step 5: Commit** `feat(feed): chronological feed and now-travelling row`

---

### Task 5: HTTP, wiring, docs

**Files:**
- Create: `internal/handlers/block_handler.go`, `internal/handlers/report_handler.go`, `internal/handlers/feed_handler.go`, `internal/app/feed_http_test.go`
- Modify: `internal/handlers/errors.go`, `internal/handlers/user_handler.go`, `internal/routes/routes.go`, `internal/app/app.go`, `README.md`

**Interfaces:**
- Routes (ใต้ `/api/v1`):
  - `POST /users/:username/block` → 204; `DELETE /users/:username/block` → 204; `GET /me/blocks` → `{"users": [...]}` (requireAuth ทั้งหมด)
  - `GET /users/:username` → optionalAuth (เดิมไม่มี auth)
  - `POST /reports` requireAuth → 201 (ใหม่) / 200 (ซ้ำ) body = `{"id","target_type","reason","status","created_at"}` (ไม่คืน target id)
  - `GET /feed?before=&limit=` requireAuth → `FeedPage`; `GET /feed/now` requireAuth → `{"travelers": [...]}`
- `errors.go`: `ErrReportNotFound` → 404
- README: ส่วน Feed / Block / Report + คำสั่ง CLI สามตัว
- [ ] **Step 1: Write the failing tests** — HTTP flow: block → 204 → follow กลับได้ 404 → `GET /me/blocks` มีชื่อ → unblock → follow ได้; ไม่ login `GET /feed` → 401; feed มี event ของคนที่ follow; `/feed/now` มีคนที่ follow ที่ ongoing; report ทริปที่เห็น → 201 แล้วซ้ำ → 200; report ทริป private ของคนอื่น → 404; `limit=abc` → 400
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement**
- [ ] **Step 4: Verify** `go vet ./... && go test ./...` → PASS
- [ ] **Step 5: Commit** `feat(feed): HTTP endpoints for feed, blocks and reports`
