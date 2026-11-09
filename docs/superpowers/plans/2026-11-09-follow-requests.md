# Follow Requests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Following someone who has "approve followers" on (the default) creates a request they accept or decline, so strangers can't become followers and see followers-only trips just by clicking Follow.

**Architecture:** A new `follow_requests` table holds pending requests. The `follows` table keeps meaning "accepted follower", so AccessService, PrivacyService, feed and profile code don't change. FollowService gains request/accept/decline/remove-follower logic. Users get a `follow_approval` setting (default true) editable via `PATCH /me`.

**Tech Stack:** Go 1.24.1, Fiber v2, GORM (Postgres prod, SQLite tests).

**Spec:** `docs/business-requirements/2026-11-08-social-trip-sharing.md` §7.7 says "follow instantly, no approval *(my call)*". This plan reverses that call. Reason: followers-only is the default trip visibility, so instant follow lets any stranger see when someone is away (spec §6 privacy). Update §7.7 and §12 open questions in Task 3.

## Global Constraints

- Do not bump the Go version or add dependencies.
- `follows` rows keep meaning "accepted". A pending requester is never a follower anywhere.
- Block-related users behave as if the other doesn't exist: 404, never listed.
- Responses use the existing envelope (`utils.Success`) and `respondError`.
- Lists return `[]`, never `null`.
- Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Rulings (controller decisions)

- `follow_approval` defaults to **true** for new and existing users (privacy first). Existing follows stay.
- Turning `follow_approval` off accepts every pending request (as Instagram does when an account goes public).
- Pending outgoing requests are capped at **200** per user. Over the cap → `ErrTooManyRequests` (already maps to 429).
- A declined requester may ask again. No cooldown (YAGNI).

## Review Focus

1. A pending requester must not see followers-only trip details, the live status, or moments. Prove it with an HTTP test.
2. A block must delete requests in both directions. Accepting after a block must return 404.
3. GORM skips zero values on Create for fields with `default:true`. Updating `follow_approval` to false must use an explicit column update, not `Save`/`Updates(struct)`.
4. Following a user who already accepted you (already in `follows`) must not create a request.
5. Turning approval off must accept requests atomically, and must skip requesters now blocked.

---

### Task 1: Model, migration and repository

**Files:**
- Create: `internal/models/follow_request.go`
- Modify: `internal/models/user.go` (add `FollowApproval`), `database/` models list (register `FollowRequest`)
- Modify: `internal/repository/follow_repository.go`, `internal/repository/follow_gorm.go`
- Modify: `internal/repository/block_gorm.go` (`BlockAndUnfollow` also deletes requests both ways)
- Test: `internal/repository/follow_gorm_test.go`, `internal/repository/block_gorm_test.go`

**Interfaces — Produces:**
- `models.FollowRequest{RequesterID string (varchar(10), PK), TargetID string (varchar(10), PK, index), CreatedAt time.Time}`
- `models.User.FollowApproval bool` — `gorm:"not null;default:true" json:"follow_approval"`
- `models.ProfileUpdate.FollowApproval *bool` — `json:"follow_approval"`
- `models.FollowEntry{User PublicProfile json:"user"; CreatedAt time.Time json:"created_at"}` (in `follow_request.go`)
- New `FollowRepository` methods:
  - `Request(ctx, requesterID, targetID string) error` — idempotent; keeps the original `created_at`.
  - `CancelRequest(ctx, requesterID, targetID string) error` — idempotent.
  - `HasRequest(ctx, requesterID, targetID string) (bool, error)`
  - `CountOutgoingRequests(ctx, requesterID string) (int64, error)`
  - `AcceptRequest(ctx, requesterID, targetID string) (bool, error)` — in one transaction: delete the request; if a row was deleted, insert the follow (on conflict do nothing). Returns false when there was no request.
  - `AcceptAllRequests(ctx, targetID string, skip map[string]bool) error` — one transaction: insert follows for every requester not in `skip`, then delete all requests to `targetID`.
  - `ListRequests(ctx, targetID string, limit int) ([]models.FollowEntry, error)` — newest first, joined with users.
  - `CountRequests(ctx, targetID string) (int64, error)`
  - `ListFollowers(ctx, userID string, limit int) ([]models.FollowEntry, error)` — newest first.
- `UserRepository` gains `SetFollowApproval(ctx, userID string, on bool) error` (explicit `Update("follow_approval", on)`).

- [ ] **Step 1: Write failing repo tests:** request is idempotent and keeps `created_at`; cancel; `AcceptRequest` returns true and creates the follow, a second call returns false; `AcceptAllRequests` skips ids in `skip` and deletes all requests; list order newest first with user fields filled; `ListFollowers`; `BlockAndUnfollow` deletes requests in both directions; a new user's `FollowApproval` is true; `SetFollowApproval(false)` persists false.
- [ ] **Step 2: Run** `go test ./internal/repository/...` — expect FAIL (undefined).
- [ ] **Step 3: Implement** the model, the migration registration and the repo methods.
- [ ] **Step 4: Run** `go vet ./... && go test ./...` — expect PASS.
- [ ] **Step 5: Commit** `feat(follow): follow_requests table, follow_approval setting, repo methods`

### Task 2: Service rules, profile fields and `/me` setting

**Files:**
- Modify: `internal/services/follow_service.go`, `internal/services/profile_service.go`, `internal/services/user_service.go`
- Test: `internal/services/follow_service_test.go` (create)

**Interfaces — Consumes:** Task 1 repo methods. **Produces:**
- `type FollowState struct { IsFollowing bool json:"is_following"; Requested bool json:"requested"; Followers int64 json:"followers" }`
- `FollowService.Follow(ctx, followerID, username) (FollowState, error)`:
  - self → ValidationError; block-related → `ErrUserNotFound`.
  - Already following → state with `IsFollowing=true`.
  - Target has `FollowApproval` on → if a request already exists, return `Requested=true`. Otherwise, if outgoing ≥ 200 → `ErrTooManyRequests`; else `Request`, then `Requested=true`.
  - Else → `Follow`, `IsFollowing=true`.
- `FollowService.Unfollow(ctx, followerID, username) (FollowState, error)` — deletes the follow and cancels the request.
- `FollowService.Requests(ctx, userID) ([]models.FollowEntry, error)` — limit 100, drops block-related users.
- `FollowService.Accept(ctx, userID, requesterUsername) error` — unknown user, no request, or block-related → `ErrUserNotFound`.
- `FollowService.Decline(ctx, userID, requesterUsername) error` — idempotent; unknown user → `ErrUserNotFound`.
- `FollowService.Followers(ctx, userID) ([]models.FollowEntry, error)` — limit 200, drops block-related users.
- `FollowService.RemoveFollower(ctx, userID, followerUsername) error` — deletes the follow `follower→user`; idempotent.
- `FollowService.RequestCount(ctx, userID) (int64, error)`
- `ProfileView` gains `FollowRequested bool json:"follow_requested"` (viewer has a pending request) and `FollowApproval bool json:"follow_approval"` (the profile user's setting). Neither is set for the blocked view.
- `UserService.UpdateProfile`: when `in.FollowApproval != nil`, call `SetFollowApproval`. When it is switched from true to false, call `AcceptAllRequests` with the target's block-related ids as `skip` (via `BlockRepository.RelatedIDs`, when blocks are wired). Wire the follow repo into UserService with a `WithFollows` option, following the existing `WithBlocks` pattern.

- [ ] **Step 1: Write failing service tests** for every bullet above, including the 200 cap, "already following → no request", and "switch off accepts all but blocked".
- [ ] **Step 2: Run** — expect FAIL.
- [ ] **Step 3: Implement.** Keep `Counts` as is.
- [ ] **Step 4: Run** `go vet ./... && go test ./...` — expect PASS.
- [ ] **Step 5: Commit** `feat(follow): request/accept/decline/remove-follower rules and approval setting`

### Task 3: HTTP routes, privacy test, docs

**Files:**
- Modify: `internal/handlers/follow_handler.go`, `internal/routes/routes.go`, `internal/app/app.go` (wiring), `internal/handlers/user_handler.go` (only if `/me` needs the count)
- Test: `internal/app/follow_http_test.go` (create)
- Modify: `README.md` (API section), BRD §7.7 and §12

**Routes (all `requireAuth`, static `/me/*` paths before params):**
- `POST /users/:username/follow` → `FollowState`
- `DELETE /users/:username/follow` → `FollowState`
- `GET /me/follow-requests` → `{"items": [FollowEntry], "count": n}`
- `POST /me/follow-requests/:username/accept` → `{"accepted": true}`
- `DELETE /me/follow-requests/:username` → `{"declined": true}`
- `GET /me/followers` → `{"items": [FollowEntry]}`
- `DELETE /me/followers/:username` → `{"removed": true}`
- `GET /me` keeps returning the user; the user JSON now carries `follow_approval`.

- [ ] **Step 1: Write failing HTTP tests:**
  - With approval on, follow gives `requested:true`. The requester then gets the redacted (non-follower) view of a followers-only ongoing trip, and the moments list is not readable.
  - Accept → the requester sees the full trip.
  - Decline → nothing changes. Cancel through `DELETE /follow` → the owner's list is empty.
  - Remove follower → access goes back to the stranger view.
  - `PATCH /me {"follow_approval": false}` accepts the pending requests, and following afterwards is instant.
  - Block → both lists are empty and accept returns 404.
  - Profile shows `follow_requested` and `follow_approval`.
- [ ] **Step 2: Run** — expect FAIL.
- [ ] **Step 3: Implement** the handlers, routes and wiring.
- [ ] **Step 4: Run** `go vet ./... && go test ./...` — expect PASS.
- [ ] **Step 5:** Update the README API table and the BRD (§7.7: approval on by default, can be turned off; §12: mark the follow question decided by Claude, reversible).
- [ ] **Step 6: Commit** `feat(follow): follow request endpoints, followers list, docs`
