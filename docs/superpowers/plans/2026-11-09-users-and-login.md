# Users & Login Implementation Plan (MVP แผน 1 จาก 6)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** เพิ่มระบบ user, login (Google + email link), profile และเจ้าของทริป ให้ `journey-service` เพื่อเป็นฐานของฟีเจอร์โซเชียลทั้งหมด

**Architecture:** ทำตาม layer เดิมของโปรเจค (Handler → Service → Repository → GORM) เพิ่ม `internal/app` ไว้ประกอบ fiber app ให้ทั้ง `main.go` และ test ใช้ตัวเดียวกัน session เป็น token สุ่มส่งผ่าน header `Authorization: Bearer <token>` ฝั่ง DB เก็บแค่ SHA-256 hash ของ token ส่วน Google และ email อยู่หลัง interface เพื่อให้ test ใส่ตัวปลอมได้

**Tech Stack:** Go 1.24, Fiber v2, GORM + PostgreSQL, `github.com/glebarez/sqlite` v1.11.0 (pure-Go SQLite ใช้ใน test เท่านั้น), `google.golang.org/api/idtoken` (ตรวจ Google ID token), `net/smtp`

**Spec:** `docs/business-requirements/2026-11-08-social-trip-sharing.md` (ข้อ 7.2, 7.3, 10, 11)

## Roadmap ของ MVP

แผนนี้เป็นแผนแรก แผนที่เหลือเขียนทีละแผนหลังแผนก่อนหน้าเสร็จ

| # | แผน | ครอบคลุม spec ข้อ |
|---|---|---|
| **1** | **Users & Login (แผนนี้)** | 7.2 (ส่วน account/profile พื้นฐาน), 7.3 (owner), 10 |
| 2 | Trip members & แก้ทริปทีละส่วน | 7.3, 7.4 |
| 3 | Visibility, trip status, privacy, "กำลังเที่ยว" | 6, 7.1, 7.5, 7.2 (หน้า profile เต็ม) |
| 4 | Moments (รูป/วิดีโอ) | 7.6 |
| 5 | Follow, feed, block/report | 6.2 ข้อ 7, 7.7 |
| 6 | Web frontend + share link preview | 7.8, 8, 11 (repo แยก) |

## Global Constraints

- ทุก response ใช้ `utils.Success` / `utils.Error` ใน `pkg/utils/response.go` เหมือนเดิม
- ID ของ record ใหม่ทุกตัวใช้ `newShortID()` ใน `internal/models/journey.go` (nanoid 10 ตัว, `0-9a-z`)
- Login: Google หรือ email link เท่านั้น ไม่มีรหัสผ่าน (spec 7.2)
- Session อายุ 30 วัน · email login link อายุ 15 นาที ใช้ได้ครั้งเดียว
- ขอ email link ได้ไม่เกิน 5 ครั้ง ต่อ email ต่อ 1 ชั่วโมง
- `username`: `^[a-z0-9_]{3,20}$` ไม่ซ้ำ (ระบบแปลงเป็นตัวเล็กก่อนตรวจ)
- `display_name` ไม่เกิน 50 ตัวอักษร · `bio` ไม่เกิน 160 ตัวอักษร (นับเป็น rune)
- Email เก็บแบบ trim + ตัวเล็กเสมอ
- ห้ามเก็บ token ดิบลง DB เก็บแค่ hash
- ห้าม commit ไฟล์ `.env` และอย่า commit การแก้ `.env.example` ที่มีรหัส Postgres จริงค้างอยู่ใน working tree ตอนนี้
- Commit message ใช้ Conventional Commits ภาษาอังกฤษ เหมือน history เดิม (`feat:`, `fix:`)

## Review Focus

1. **CORS ต้องยอม header `Authorization` และ method `PATCH`** ตอนนี้ `AllowHeaders` ไม่มี `Authorization` ถ้าไม่แก้ เว็บจะเรียก endpoint ที่ต้อง login ไม่ได้เลย → test ใน Task 6
2. **Email ตัวใหญ่/มีช่องว่าง** `"  Kip@Gmail.com "` กับ `"kip@gmail.com"` ต้องเป็น user คนเดียวกัน → test ใน Task 4
3. **Login link ถูกใช้ซ้ำหรือหมดอายุ** ต้องได้ 400 และไม่สร้าง session → test ใน Task 4
4. **แก้ทริปแล้ว `owner_id` ต้องไม่หาย** `tx.Save` เขียนทับทุก field ถ้า service ไม่ copy `OwnerID` จากของเดิม เจ้าของจะแก้ทริปตัวเองไม่ได้อีก → test ใน Task 7
5. **ทริปเก่าที่ไม่มีเจ้าของ** (สร้างก่อนมีระบบ user) ยังเปิดดูได้ แต่แก้/ลบไม่ได้ (403) → test ใน Task 7

---

## File Structure

| ไฟล์ | หน้าที่ |
|---|---|
| `internal/testutil/db.go` | สร้าง SQLite in-memory + migrate สำหรับ test |
| `internal/models/user.go` | `User`, `Session`, `LoginToken`, `ProfileUpdate`, `PublicProfile` |
| `internal/auth/token.go` | สร้าง token สุ่ม และ hash |
| `internal/auth/google.go` | `GoogleVerifier` interface + ตัวจริงที่ใช้ `idtoken` |
| `internal/mailer/mailer.go` | `Mailer` interface, `LogMailer`, `SMTPMailer` |
| `internal/repository/user_repository.go`, `user_gorm.go` | อ่าน/เขียน users |
| `internal/repository/auth_repository.go`, `auth_gorm.go` | อ่าน/เขียน sessions และ login tokens |
| `internal/services/user_service.go` | profile + สร้าง username อัตโนมัติ |
| `internal/services/auth_service.go` | login ทั้งสองแบบ, logout, ตรวจ session |
| `internal/middleware/auth.go` | `RequireAuth`, `CurrentUserID` |
| `internal/handlers/auth_handler.go`, `user_handler.go` | HTTP ของ auth และ profile |
| `internal/app/app.go` | ประกอบ fiber app จาก dependency |
| แก้ `database/postgres.go`, `config/config.go`, `main.go`, `internal/routes/routes.go`, `internal/middleware/middleware.go`, `internal/models/journey.go`, `internal/services/journey_service.go`, `internal/repository/journey_*.go`, `internal/handlers/journey_handler.go`, `README.md`, `.env.example` (เฉพาะ key ใหม่) | |

---

### Task 1: Test harness + characterization test ของ journey เดิม

ตอนนี้โปรเจคยังไม่มี test เลย task นี้วางฐานให้ task อื่น

**Files:**
- Create: `internal/testutil/db.go`
- Modify: `database/postgres.go` (แยก list model ออกมา)
- Test: `internal/repository/journey_gorm_test.go`

**Interfaces:**
- Produces:
  - `database.Models() []any` คืนทุก model ที่ต้อง migrate (task หลัง ๆ เพิ่ม model ที่นี่ที่เดียว) และ `database.Migrate` เรียก `db.AutoMigrate(Models()...)`
  - `testutil.NewTestDB(t *testing.T) *gorm.DB` เปิด `sqlite.Open("file:<name>?mode=memory&cache=shared&_pragma=foreign_keys(1)")` โดย `<name>` = `t.Name()` ที่แทน `/` ด้วย `_` (แต่ละ test ได้ DB ของตัวเอง) แล้วเรียก `database.Migrate`

- [ ] **Step 1:** `go get github.com/glebarez/sqlite@v1.11.0 github.com/stretchr/testify@v1.11.1`
- [ ] **Step 2: Write the failing test** `TestJourneyRepository_CreateAndGetBySlug`
  - สร้าง Journey `{Title: "Japan Trip", Destination: "Tokyo, Japan", DepartureDate, ReturnDate}` มี 1 Day และ 2 Plan (`time` `"19:00"` และ `"14:00"`)
  - assert: `Slug == "tokyo-japan-" + ID`, `GetBySlug` คืน Day 1 วัน, Plan เรียง `"14:00"` ก่อน `"19:00"`
  - `GetBySlug("nope")` คืน `nil, nil`
- [ ] **Step 3:** Run `go test ./internal/repository/ -run TestJourneyRepository -v` → FAIL (`testutil` ยังไม่มี)
- [ ] **Step 4:** Implement `database.Models()` และ `testutil.NewTestDB`
- [ ] **Step 5:** Run ซ้ำ → PASS และ `go test ./...` ผ่านทั้งหมด
- [ ] **Step 6: Commit** `test: add sqlite test harness and journey repository test`

---

### Task 2: User models + repositories

**Files:**
- Create: `internal/models/user.go`, `internal/auth/token.go`, `internal/repository/user_repository.go`, `internal/repository/user_gorm.go`, `internal/repository/auth_repository.go`, `internal/repository/auth_gorm.go`
- Modify: `database/postgres.go` (`Models()` เพิ่ม `User`, `Session`, `LoginToken`)
- Test: `internal/auth/token_test.go`, `internal/repository/user_gorm_test.go`, `internal/repository/auth_gorm_test.go`

**Interfaces:**
- Produces (models, ทุกตัวมี `BeforeCreate` ใส่ `ID` ด้วย `newShortID()` ถ้าว่าง):
  ```go
  type User struct {
      ID          string    `gorm:"type:varchar(10);primaryKey" json:"id"`
      Email       string    `gorm:"uniqueIndex;not null" json:"email"`
      GoogleSub   *string   `gorm:"uniqueIndex" json:"-"`
      Username    string    `gorm:"type:varchar(20);uniqueIndex;not null" json:"username"`
      DisplayName string    `gorm:"type:varchar(50)" json:"display_name"`
      AvatarURL   string    `json:"avatar_url"`
      Bio         string    `gorm:"type:varchar(160)" json:"bio"`
      CreatedAt   time.Time `json:"created_at"`
      UpdatedAt   time.Time `json:"updated_at"`
  }
  type Session struct {
      ID        string    `gorm:"type:varchar(10);primaryKey"`
      UserID    string    `gorm:"type:varchar(10);not null;index"`
      TokenHash string    `gorm:"type:char(64);uniqueIndex;not null"`
      ExpiresAt time.Time `gorm:"not null"`
      CreatedAt time.Time
  }
  type LoginToken struct {
      ID        string     `gorm:"type:varchar(10);primaryKey"`
      Email     string     `gorm:"not null;index"`
      TokenHash string     `gorm:"type:char(64);uniqueIndex;not null"`
      ExpiresAt time.Time  `gorm:"not null"`
      UsedAt    *time.Time
      CreatedAt time.Time  `gorm:"index"`
  }
  type ProfileUpdate struct {           // ทุก field เป็น pointer: nil = ไม่แก้
      Username    *string `json:"username"`
      DisplayName *string `json:"display_name"`
      Bio         *string `json:"bio"`
      AvatarURL   *string `json:"avatar_url"`
  }
  type PublicProfile struct {           // ไม่มี email
      Username, DisplayName, AvatarURL, Bio string  // json: username, display_name, avatar_url, bio
  }
  ```
  `GoogleSub` เป็น pointer เพื่อให้ user ที่ไม่มี Google หลายคนเป็น NULL ได้โดยไม่ชน unique index
- Produces (`internal/auth/token.go`):
  - `NewToken() (string, error)` คืน 32 byte สุ่มจาก `crypto/rand` เป็น hex (64 ตัว)
  - `HashToken(token string) string` คืน SHA-256 เป็น hex (64 ตัว)
- Produces (repositories, คืน `nil, nil` เมื่อไม่เจอ เหมือน `JourneyRepository.GetBySlug`):
  ```go
  type UserRepository interface {
      Create(ctx context.Context, u *models.User) error
      GetByID(ctx context.Context, id string) (*models.User, error)
      GetByEmail(ctx context.Context, email string) (*models.User, error)
      GetByGoogleSub(ctx context.Context, sub string) (*models.User, error)
      GetByUsername(ctx context.Context, username string) (*models.User, error)
      Update(ctx context.Context, u *models.User) error
  }
  type AuthRepository interface {
      CreateSession(ctx context.Context, s *models.Session) error
      GetSessionByHash(ctx context.Context, hash string) (*models.Session, error)
      DeleteSessionByHash(ctx context.Context, hash string) error
      CreateLoginToken(ctx context.Context, t *models.LoginToken) error
      GetLoginTokenByHash(ctx context.Context, hash string) (*models.LoginToken, error)
      // ตั้ง used_at เฉพาะแถวที่ used_at IS NULL; คืน false ถ้าถูกใช้ไปแล้ว (กันกดลิงก์พร้อมกันสองที)
      MarkLoginTokenUsed(ctx context.Context, id string, at time.Time) (bool, error)
      CountLoginTokensSince(ctx context.Context, email string, since time.Time) (int64, error)
  }
  ```
  constructor: `repository.NewUserRepository(db *gorm.DB) UserRepository`, `repository.NewAuthRepository(db *gorm.DB) AuthRepository`

- [ ] **Step 1: Write failing tests**
  - `TestNewToken`: ยาว 64, เรียกสองครั้งได้ค่าไม่ซ้ำ · `TestHashToken`: `HashToken("abc") == "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"`
  - `TestUserRepository_UniqueEmailAndUsername`: สร้าง user ซ้ำ email → error; ซ้ำ username → error; สอง user ที่ `GoogleSub == nil` → สร้างได้ทั้งคู่
  - `TestUserRepository_GetBy*`: เจอด้วย email / google sub / username; ไม่เจอ → `nil, nil`
  - `TestAuthRepository_MarkLoginTokenUsed`: ครั้งแรก `true`, ครั้งที่สอง `false`
  - `TestAuthRepository_CountLoginTokensSince`: สร้าง 3 token ของ `a@x.com` (`CreatedAt` = now-2h, now-30m, now-1m) และ 1 ของ `b@x.com` → `Count("a@x.com", now-1h) == 2`
- [ ] **Step 2:** Run `go test ./internal/auth/ ./internal/repository/ -v` → FAIL
- [ ] **Step 3:** Implement models, token helpers, repositories; เพิ่ม model ใน `database.Models()`
- [ ] **Step 4:** Run ซ้ำ → PASS
- [ ] **Step 5: Commit** `feat: add user, session and login token models`

---

### Task 3: UserService (profile + username อัตโนมัติ)

**Files:**
- Create: `internal/services/user_service.go`
- Test: `internal/services/user_service_test.go`

**Interfaces:**
- Consumes: `UserRepository` (Task 2)
- Produces:
  ```go
  var ErrUsernameTaken = errors.New("username already taken")
  var ErrUserNotFound  = errors.New("user not found")
  type ValidationError struct{ Message string }   // implements error
  func NewUserService(repo repository.UserRepository) *UserService
  func (s *UserService) GetByID(ctx context.Context, id string) (*models.User, error)          // ErrUserNotFound
  func (s *UserService) GetPublicProfile(ctx context.Context, username string) (*models.PublicProfile, error) // ErrUserNotFound
  func (s *UserService) UpdateProfile(ctx context.Context, userID string, in models.ProfileUpdate) (*models.User, error)
  func (s *UserService) GenerateUsername(ctx context.Context, email string) (string, error)
  ```
- `GenerateUsername`: เอาส่วนหน้า `@` แปลงเป็นตัวเล็ก ตัดตัวที่ไม่ใช่ `[a-z0-9_]` ทิ้ง ตัดเหลือ 15 ตัว ถ้าสั้นกว่า 3 ใช้ `"traveler"` ถ้าชื่อนี้มีคนใช้แล้ว ต่อ `"_"` + 4 ตัวสุ่มจาก `0-9a-z` ลองได้สูงสุด 5 รอบ แล้วคืน error

- [ ] **Step 1: Write failing tests**
  - `TestGenerateUsername`: `"Kip.Per+trip@gmail.com"` → `"kippertrip"`; `"a@x.com"` → `"traveler"`; ถ้ามี `"kippertrip"` อยู่แล้ว → ตรง regex `^kippertrip_[a-z0-9]{4}$`
  - `TestUpdateProfile_Validation`: username `"ab"` → `ValidationError`; `"Kip_Per"` → บันทึกเป็น `"kip_per"`; `"kip per"` → `ValidationError`; display name 51 rune → `ValidationError`; bio 160 ตัวอักษรไทย → ผ่าน, 161 → `ValidationError`
  - `TestUpdateProfile_UsernameTaken`: user B ขอ username ของ A → `ErrUsernameTaken`; user A ส่ง username เดิมของตัวเอง → ผ่าน
  - `TestUpdateProfile_PartialUpdate`: ส่งแค่ `Bio` แล้ว `DisplayName` เดิมยังอยู่
  - `TestGetPublicProfile`: ไม่มี field email ใน struct ที่คืน; username ไม่มีอยู่ → `ErrUserNotFound`
- [ ] **Step 2:** Run `go test ./internal/services/ -run 'Username|Profile' -v` → FAIL
- [ ] **Step 3:** Implement
- [ ] **Step 4:** Run ซ้ำ → PASS
- [ ] **Step 5: Commit** `feat: add user service with profile validation`

---

### Task 4: AuthService — email login, session, logout

**Files:**
- Create: `internal/services/auth_service.go`, `internal/mailer/mailer.go`
- Test: `internal/services/auth_service_test.go`, `internal/testutil/fakes.go`

**Interfaces:**
- Consumes: `UserRepository`, `AuthRepository`, `auth.NewToken`, `auth.HashToken` (Task 2), `UserService.GenerateUsername` (Task 3)
- Produces (`internal/mailer`):
  ```go
  type Mailer interface { Send(ctx context.Context, to, subject, body string) error }
  func NewLogMailer() Mailer                    // พิมพ์ to/subject/body ลง zerolog (ใช้ตอน dev)
  func NewSMTPMailer(cfg config.SMTP) Mailer    // net/smtp + PlainAuth
  ```
- Produces (`internal/testutil/fakes.go`): `type CaptureMailer struct{ Sent []SentMail }` (`SentMail{To, Subject, Body string}`) implement `mailer.Mailer`
- Produces (`internal/services/auth_service.go`):
  ```go
  const SessionTTL = 30 * 24 * time.Hour
  const LoginTokenTTL = 15 * time.Minute
  const MaxLoginLinksPerHour = 5
  var ErrInvalidToken    = errors.New("invalid or expired token")
  var ErrTooManyRequests = errors.New("too many requests")
  var ErrUnauthorized    = errors.New("unauthorized")
  type AuthDeps struct {
      Users   repository.UserRepository
      Auth    repository.AuthRepository
      UserSvc *UserService
      Mailer  mailer.Mailer
      WebURL  string                // เช่น "http://localhost:5173"
      Now     func() time.Time      // nil = time.Now
  }
  func NewAuthService(d AuthDeps) *AuthService
  func (s *AuthService) StartEmailLogin(ctx context.Context, email string) error
  func (s *AuthService) VerifyEmailLogin(ctx context.Context, token string) (sessionToken string, user *models.User, err error)
  func (s *AuthService) Authenticate(ctx context.Context, sessionToken string) (*models.User, error) // ErrUnauthorized
  func (s *AuthService) Logout(ctx context.Context, sessionToken string) error
  ```
- `StartEmailLogin`: trim + ตัวเล็ก, ตรวจรูปแบบด้วย `net/mail.ParseAddress` (ผิด → `ValidationError`), ถ้านับได้ ≥ 5 ใน 1 ชั่วโมง → `ErrTooManyRequests`, สร้าง token, ส่งเมลหัวข้อ `"เข้าสู่ระบบ Journey"` เนื้อหามีลิงก์ `{WebURL}/auth/verify?token={token}` ส่งเหมือนกันทั้ง email ที่มีและไม่มี user (ไม่บอกว่ามีบัญชีหรือเปล่า)
- `VerifyEmailLogin`: หา token ด้วย hash, ไม่เจอ / หมดอายุ / `MarkLoginTokenUsed` คืน false → `ErrInvalidToken` ถ้าไม่มี user ของ email นี้ สร้างใหม่ด้วย `GenerateUsername` แล้วสร้าง session

- [ ] **Step 1: Write failing tests** (ใช้ `testutil.NewTestDB`, `CaptureMailer`, `Now` ที่เลื่อนเวลาได้)
  - `TestStartEmailLogin_SendsLink`: ส่ง 1 ฉบับถึง `kip@gmail.com` body มี `"http://web.test/auth/verify?token="`
  - `TestEmailLogin_NewUser`: ดึง token จาก body → `VerifyEmailLogin` ได้ session token ยาว 64 และ user ใหม่ `Username == "kip"`
  - `TestEmailLogin_NormalizesEmail` (Review Focus 2): login ด้วย `"  Kip@Gmail.com "` แล้วด้วย `"kip@gmail.com"` → user ID เดียวกัน, `Email == "kip@gmail.com"`
  - `TestVerifyEmailLogin_ReusedToken` (Review Focus 3): ใช้ token ครั้งที่สอง → `ErrInvalidToken`
  - `TestVerifyEmailLogin_Expired` (Review Focus 3): เลื่อนเวลา 16 นาที → `ErrInvalidToken` และไม่มีแถวใน `sessions`
  - `TestStartEmailLogin_RateLimit`: ครั้งที่ 6 ใน 1 ชั่วโมง → `ErrTooManyRequests`; เลื่อนเวลา 61 นาที → ส่งได้อีก
  - `TestStartEmailLogin_InvalidEmail`: `"not-an-email"` → `ValidationError`
  - `TestAuthenticate`: token ถูก → user; token มั่ว → `ErrUnauthorized`; เลื่อนเวลา 31 วัน → `ErrUnauthorized`
  - `TestLogout`: หลัง logout `Authenticate` → `ErrUnauthorized`
- [ ] **Step 2:** Run `go test ./internal/services/ -run 'Email|Authenticate|Logout' -v` → FAIL
- [ ] **Step 3:** Implement mailer + auth service (email ส่วน); เพิ่ม `config.SMTP` struct ใน `config/config.go`: `Host, Port, User, Password, From` จาก env `SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASSWORD, SMTP_FROM`
- [ ] **Step 4:** Run ซ้ำ → PASS
- [ ] **Step 5: Commit** `feat: add email link login and sessions`

---

### Task 5: Google login

**Files:**
- Create: `internal/auth/google.go`
- Modify: `internal/services/auth_service.go` (เพิ่ม field `Google auth.GoogleVerifier` ใน `AuthDeps`), `internal/testutil/fakes.go`
- Test: `internal/services/auth_service_google_test.go`

**Interfaces:**
- Produces (`internal/testutil/fakes.go`): `type FakeGoogle struct{ Identities map[string]*auth.GoogleIdentity }` — `Verify` คืน identity ตาม key ที่ส่งมา ไม่เจอคืน error
- Produces:
  ```go
  type GoogleIdentity struct { Subject, Email string; EmailVerified bool; Name, Picture string }
  type GoogleVerifier interface { Verify(ctx context.Context, idToken string) (*GoogleIdentity, error) }
  func NewGoogleVerifier(clientID string) GoogleVerifier   // ใช้ idtoken.Validate(ctx, token, clientID)
  func (s *AuthService) LoginWithGoogle(ctx context.Context, idToken string) (sessionToken string, user *models.User, err error)
  ```
- ลำดับ: verify (error → `ErrInvalidToken`) → `EmailVerified == false` → `ErrInvalidToken` → หา user ด้วย sub → ไม่เจอ หาด้วย email (trim + ตัวเล็ก) แล้วผูก `GoogleSub` ให้ → ไม่เจออีก สร้าง user ใหม่ ใช้ `Name` เป็น `DisplayName` (ตัดเหลือ 50 rune) และ `Picture` เป็น `AvatarURL` → สร้าง session
- user ที่มีอยู่แล้ว ห้ามเขียนทับ `DisplayName` / `AvatarURL` ที่เขาแก้เอง

- [ ] **Step 1: Write failing tests** (ใช้ `testutil.FakeGoogle`)
  - `TestGoogleLogin_NewUser`: ได้ user ใหม่ `DisplayName == "Kip Per"`, `AvatarURL` ตาม `Picture`
  - `TestGoogleLogin_ReturningUser`: login ซ้ำ → user ID เดิม, ไม่มี user เพิ่ม
  - `TestGoogleLogin_LinksExistingEmailUser`: user สมัครด้วย email ไว้ก่อน → login Google ด้วย email เดียวกัน (ตัวใหญ่) → user ID เดิม และ `GoogleSub` ถูกตั้ง
  - `TestGoogleLogin_KeepsEditedProfile`: user แก้ display name เป็น `"Kipper"` แล้ว login Google อีกรอบ → ยังเป็น `"Kipper"`
  - `TestGoogleLogin_UnverifiedEmail`: `EmailVerified: false` → `ErrInvalidToken`
  - `TestGoogleLogin_BadToken`: token ที่ fake ไม่รู้จัก → `ErrInvalidToken`
- [ ] **Step 2:** Run `go test ./internal/services/ -run Google -v` → FAIL
- [ ] **Step 3:** `go get google.golang.org/api/idtoken` แล้ว implement
- [ ] **Step 4:** Run ซ้ำ → PASS
- [ ] **Step 5: Commit** `feat: add google login`

---

### Task 6: HTTP — middleware, handlers, routes, app wiring

**Files:**
- Create: `internal/middleware/auth.go`, `internal/handlers/auth_handler.go`, `internal/handlers/user_handler.go`, `internal/app/app.go`
- Modify: `internal/middleware/middleware.go` (CORS), `internal/routes/routes.go`, `config/config.go`, `main.go`, `.env.example` (เพิ่ม key ใหม่แบบค่าว่าง)
- Test: `internal/app/auth_http_test.go`

**Interfaces:**
- Consumes: `AuthService`, `UserService` (Task 3–5), `testutil.*`
- Produces:
  ```go
  // internal/middleware/auth.go
  func RequireAuth(svc *services.AuthService) fiber.Handler  // อ่าน "Authorization: Bearer <t>", ไม่ผ่าน → 401 "unauthorized"
  func CurrentUserID(c *fiber.Ctx) string                    // ค่าจาก c.Locals("user_id")
  // internal/app/app.go
  type Deps struct {
      DB     *gorm.DB
      Google auth.GoogleVerifier
      Mailer mailer.Mailer
      WebURL string
      Now    func() time.Time
  }
  func New(d Deps) *fiber.App   // สร้าง repo/service/handler ทั้งหมด + middleware.Register + routes.Register
  ```
- Config เพิ่ม: `AppConfig.WebURL` (`APP_WEB_URL`), `Google.ClientID` (`GOOGLE_CLIENT_ID`), `SMTP` (Task 4) · `main.go` ใช้ `app.New` และเลือก `NewLogMailer()` เมื่อ `SMTP_HOST` ว่าง
- CORS: `AllowHeaders: "Origin, Content-Type, Accept, Authorization"`, `AllowMethods: "GET, POST, PUT, PATCH, DELETE, OPTIONS"`
- Endpoints (prefix `/api/v1`):

  | Method + path | Auth | Body | สำเร็จ |
  |---|---|---|---|
  | `POST /auth/google` | – | `{"id_token"}` | 200 `{"token", "user"}` |
  | `POST /auth/email/start` | – | `{"email"}` | 200 `{}` |
  | `POST /auth/email/verify` | – | `{"token"}` | 200 `{"token", "user"}` |
  | `POST /auth/logout` | ✅ | – | 200 `{}` |
  | `GET /me` | ✅ | – | 200 `User` (มี email) |
  | `PATCH /me` | ✅ | `ProfileUpdate` | 200 `User` |
  | `GET /users/:username` | – | – | 200 `PublicProfile` |

  แปลง error: `ValidationError` → 400 (message ของมัน) · `ErrInvalidToken` → 400 · `ErrUnauthorized` → 401 · `ErrUserNotFound` → 404 · `ErrUsernameTaken` → 409 · `ErrTooManyRequests` → 429 · อื่น ๆ → 500

- [ ] **Step 1: Write failing tests** (สร้าง app ด้วย `app.New` + SQLite + fakes แล้วยิงด้วย `app.Test`)
  - `TestHTTP_EmailLoginFlow`: start → ดึง token จาก `CaptureMailer` → verify → `GET /me` ด้วย Bearer ได้ email `kip@gmail.com`
  - `TestHTTP_MeWithoutToken`: `GET /me` ไม่มี header → 401 และ `message == "unauthorized"`
  - `TestHTTP_PatchMe`: เปลี่ยน username เป็น `"kipper"` → 200; อีก user ขอ `"kipper"` → 409; username `"x"` → 400
  - `TestHTTP_PublicProfileHidesEmail`: `GET /users/kipper` → body ไม่มีคำว่า `"email"`
  - `TestHTTP_Logout`: logout แล้ว `GET /me` ด้วย token เดิม → 401
  - `TestHTTP_RateLimit`: start ครั้งที่ 6 → 429
  - `TestHTTP_CORSPreflight` (Review Focus 1): `OPTIONS /api/v1/me` พร้อม `Origin: http://web.test`, `Access-Control-Request-Method: PATCH`, `Access-Control-Request-Headers: authorization` → header `Access-Control-Allow-Headers` มี `Authorization` และ `Access-Control-Allow-Methods` มี `PATCH`
- [ ] **Step 2:** Run `go test ./internal/app/ -v` → FAIL
- [ ] **Step 3:** Implement middleware, handlers, routes (เส้นทาง journey เดิมยังต่อไว้เหมือนเดิมใน task นี้), `app.New`, config, `main.go`
- [ ] **Step 4:** Run `go test ./... ` → PASS และ `go build ./...` ผ่าน
- [ ] **Step 5: Commit** `feat: add auth and profile endpoints`

---

### Task 7: เจ้าของทริป + ปิด endpoint อันตราย

**Files:**
- Modify: `internal/models/journey.go`, `internal/services/journey_service.go`, `internal/handlers/journey_handler.go`, `internal/repository/journey_repository.go`, `internal/repository/journey_gorm.go`, `internal/routes/routes.go`, `README.md`
- Test: `internal/app/journey_http_test.go`

**Interfaces:**
- Consumes: `middleware.RequireAuth`, `middleware.CurrentUserID`, `app.New` (Task 6)
- Produces:
  - `Journey.OwnerID string` — `gorm:"type:varchar(10);index" json:"owner_id"` (ว่างได้ สำหรับทริปเก่า)
  - `var ErrForbidden = errors.New("forbidden")` ใน `services`
  - `JourneyService.Create(ctx, ownerID string, j *models.Journey) error`
  - `JourneyService.Update(ctx, userID, slug string, j *models.Journey) error` — `ErrNotFound` / `ErrForbidden`; copy `ID`, `Slug`, `CreatedAt`, **`OwnerID`** จากของเดิม
  - `JourneyService.Delete(ctx, userID, slug string) error` — หาของเดิมก่อน คืน `ErrNotFound` / `ErrForbidden`
  - เจ้าของ = `existing.OwnerID != "" && existing.OwnerID == userID` ทริปที่ `OwnerID == ""` ไม่มีใครแก้ได้
  - ลบ `DeleteAll` ออกจาก repository, service, handler และ route
  - Routes: `POST /journeys/create`, `PUT /journeys/:slug/update`, `DELETE /journeys/:slug` ต้องผ่าน `RequireAuth` · `GET` ยังเปิดสาธารณะ (visibility มาในแผน 3)
  - Handler: `ErrForbidden` → 403 `"forbidden"`

- [ ] **Step 1: Write failing tests**
  - `TestHTTP_CreateJourneyRequiresLogin`: ไม่มี token → 401; มี token → 201 และ `GET /journeys/:slug` ได้ `owner_id` = user นั้น
  - `TestHTTP_OnlyOwnerCanUpdate`: user B update ทริปของ A → 403; A update → 200
  - `TestHTTP_UpdateKeepsOwner` (Review Focus 4): A update สองครั้งติดกัน → ทั้งสองครั้ง 200 และ `owner_id` ยังเป็น A
  - `TestHTTP_OnlyOwnerCanDelete`: B ลบ → 403; A ลบ → 200; ลบ slug ที่ไม่มี → 404 (เดิมได้ 500)
  - `TestHTTP_LegacyJourneyReadOnly` (Review Focus 5): ใส่ journey ที่ `OwnerID == ""` ตรงเข้า DB → `GET` 200, `PUT` 403, `DELETE` 403
  - `TestHTTP_DeleteAllRemoved`: `DELETE /api/v1/journeys` → 404 หรือ 405
- [ ] **Step 2:** Run `go test ./internal/app/ -run Journey -v` → FAIL
- [ ] **Step 3:** Implement
- [ ] **Step 4:** Run `go test ./...` → PASS และ `go build ./...` ผ่าน
- [ ] **Step 5:** อัปเดต `README.md`: เพิ่มหัวข้อ Auth (endpoint ตารางใน Task 6 + วิธีส่ง Bearer), เพิ่ม env ใหม่ในตาราง Configuration, แก้ path ของ Update/Delete ให้ตรงกับ route จริง, ลบ Delete all
- [ ] **Step 6: Commit** `feat: add journey ownership and remove delete-all endpoint`

---

## สิ่งที่แผนนี้ตัดสินใจเอง (ไม่ได้มาจาก spec)

- Session เป็น token สุ่มเก็บ hash ใน DB แทน JWT เพราะ logout ได้ทันทีและไม่ต้องจัดการ key
- ส่ง token ผ่าน header `Authorization` แทน cookie เพราะเว็บกับ API อาจอยู่คนละ domain (Firebase Hosting กับ Cloud Run ตาม `Makefile`)
- Test ใช้ SQLite in-memory แทน Postgres จริง เพื่อให้รัน `go test ./...` ได้ทันทีโดยไม่ต้องเปิด database
- ทริปเก่าที่ไม่มีเจ้าของ แก้/ลบไม่ได้ ถ้าต้องการโอนเป็นของใคร ให้ update `owner_id` ใน DB ด้วยมือ
- ลบ endpoint "ลบทุกทริป" ทิ้งเลย ไม่ได้ทำเป็น admin-only
