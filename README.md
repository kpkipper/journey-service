# journey-service

## Table of Contents

- [Overview](#overview)
- [How It Works](#how-it-works)
- [Auth](#auth)
- [API Reference](#api-reference)
- [Members](#members)
- [Itinerary editing](#itinerary-editing)
- [Privacy](#privacy)
- [Profile and Follow](#profile-and-follow)
- [Moments](#moments)
- [Response Format](#response-format)
- [Project Structure](#project-structure)
- [Configuration](#configuration)
- [Getting Started](#getting-started)

---

## Overview

Each **Journey** has multiple **Days**, and each **Day** has multiple **Plans** (activities).

```
Journey ──── ItineraryDay ──── Plan
```

- **Journey** — trip info: title, destination, country, travel dates
- **ItineraryDay** — one day in the trip, sorted by date
- **Plan** — one activity in a day (time, description, emoji, map link), sorted by time

---

## How It Works

Every request goes through three layers:

```
Request → Handler → Service → Repository → Database
```

- **Handler** — reads the request, validates input, returns JSON
- **Service** — handles business logic
- **Repository** — talks to the database

---

## Auth

All paths below are under the `/api/v1` prefix. Login is Google or an email link only (no passwords). A successful login returns a random session `token` (valid 30 days; only its hash is stored). Send it on protected requests:

```
Authorization: Bearer <token>
```

| Method | Path                    | Auth | Description                                               |
| ------ | ----------------------- | ---- | --------------------------------------------------------- |
| POST   | `/api/v1/auth/google`          | no   | log in with a Google ID token                             |
| POST   | `/api/v1/auth/email/start`     | no   | email a login link (valid 15 min, one use, 5 per hour)    |
| POST   | `/api/v1/auth/email/verify`    | no   | exchange the emailed token for a session token            |
| POST   | `/api/v1/auth/logout`          | yes  | revoke the current session                                |
| GET    | `/api/v1/me`                   | yes  | current user                                              |
| PATCH  | `/api/v1/me`                   | yes  | update `username`, `display_name`, `bio`, `avatar_url` (https, max 2048 bytes), `follow_approval` (bool) |
| GET    | `/api/v1/users/:username`      | optional | public profile (no email); `404` if the viewer is block-related |

Journeys have an owner (`owner_id`, set from the logged-in user on create). Creating, updating and deleting a journey requires login; only the owner may update or delete it (`403 forbidden` if the trip is visible to the caller but they are not the owner, `404` if the trip is hidden from them). Who may read a journey, and how much of it, follows the [Privacy](#privacy) rules. Journeys created before login existed have an empty `owner_id` and are read-only; to hand them to a user run `./server assign-legacy-owner <username>` (see [Claiming existing trips](#claiming-existing-trips)).

---

## API Reference

Base URL: `/api/v1`

---

### Create a Journey

```
POST /journeys/create
```

Requires login. The owner is the logged-in user (any `owner_id` in the body is ignored).

**Request body:**

```json
{
  "title": "Japan Trip 2026",
  "destination": "Tokyo, Japan",
  "country": "Japan",
  "departure_date": "2026-06-01T00:00:00Z",
  "return_date": "2026-06-07T00:00:00Z",
  "itinerary_days": [
    {
      "date": "Day 1",
      "date_iso": "2026-06-01T00:00:00Z",
      "title": "Arrival & Shinjuku",
      "plans": [
        {
          "time": "14:00",
          "description": "Check in hotel",
          "country": "Japan",
          "emoji": "🏨",
          "map_url": "https://maps.google.com/?q=shinjuku"
        },
        {
          "time": "19:00",
          "description": "Dinner at Omoide Yokocho",
          "country": "Japan",
          "emoji": "🍜",
          "map_url": "https://maps.google.com/?q=omoide+yokocho"
        }
      ]
    }
  ]
}
```

**Required fields:**

| Field                       | Required |
| --------------------------- | -------- |
| `title`                     | ✅       |
| `destination`               | ✅       |
| `departure_date`            | ✅       |
| `return_date`               | ✅       |
| `itinerary_days[].date`     | ✅       |
| `itinerary_days[].date_iso` | ✅       |
| `plans[].description`       | ✅       |
| everything else             | optional |

**Response:** `201 Created`

---

### List all Journeys

```
GET /journeys
```

Returns journeys grouped by country. Journeys without a country are grouped under `"Other"`. Only trips the viewer may see in lists appear (private trips and other people's `link` trips never do). Each item carries `cover_url` and the viewer's `status`. The title is `"<destination> <year>"`; for a trip the viewer sees redacted it is the destination only, with no year.

**Response:** `200 OK`

```json
{
  "code": "JOURNEY-200000",
  "message": "Success",
  "data": [
    {
      "country": "Japan",
      "plan": [
        {
          "id": "uuid",
          "slug": "tokyo-abc123",
          "title": "Tokyo 2026",
          "destination": "Tokyo, Japan",
          "cover_url": "https://...",
          "status": {"state": "completed", "days_until": 0, "day_number": 0, "total_days": 5}
        }
      ]
    }
  ]
}
```

---

### Get a Journey

```
GET /journeys/:slug
```

Login is optional; what the response contains depends on the viewer (see [Privacy](#privacy)). A trip the viewer may not see is `404 journey not found`.

**Response:** `200 OK`, `data` has:

- always: `id`, `slug`, `title`, `destination`, `country`, `cover_url`, `visibility`, `status` (`{state, days_until, day_number, total_days}`), `role` (the viewer's role: `owner`, `editor`, `traveler` or empty), `redacted`, and `owner` (`username, display_name, avatar_url, bio`; absent for trips created before login existed, which are shown as `public`).
- when `redacted` is `false`: also `month_start`/`month_end` (`YYYY-MM`), `departure_date`, `return_date`, `updated_at`, `itinerary_days` (all days with their plans; hidden plans only for members) and `members` (the owner and every joined member: `username`, `display_name`, `avatar_url`, `role`, `status`; pending invites are not listed), and `live` while the trip is on now.
- **members only** (`role` is `owner`, `editor` or `traveler`): `settings` (`{is_draft, timezone, hide_live_status, editors_can_invite, members_can_post}`). Absent for followers and strangers, who must not learn the owner's draft state or live-status choice.
- **Strangers** (not a member, not a follower) get `redacted: true` until the trip is completed: only the "always" fields, with `status` just `planning` or `upcoming` (counters `0`) and no dates, days, members or `live`. After completion they see the full view like followers.

---

### Update a Journey

```
PUT /journeys/:slug/update
```

Requires login; **owner only** (`403` for editors and other members or viewers who can see the trip, `404` if the trip is hidden from the caller).

Updates only `title`, `destination`, `country`, `departure_date` and `return_date`, and replaces all days and plans. `cover_url` and `editors_can_invite` are not touched by this endpoint: change them with `PATCH /journeys/:slug`. A deleted trip is `404`. Make sure to include every day and plan you want to keep — anything not sent will be deleted. Days and plans are re-created with fresh IDs on every call, so IDs from an earlier `GET` are stale afterwards. Prefer the fine-grained endpoints under [Itinerary editing](#itinerary-editing) for small changes. Note that a full `PUT` cannot un-hide a hotel plan (plans of category `stay` or emoji `🏨` are hidden automatically): use the plan PATCH with `hidden_from_public: false`.

**Request body:** same as Create

**Response:** `200 OK`

---

### Delete a Journey

```
DELETE /journeys/:slug
```

Requires login; owner only. Returns `404` if the journey does not exist. All of the trip's [moments](#moments) and their files are removed with it.

**Response:** `200 OK`

```json
{
  "code": "JOURNEY-200000",
  "message": "Success",
  "data": {}
}
```

---

## Members

A trip has one **owner** (`journeys.owner_id`) and up to 19 other people (owner + joined + invited is capped at **20**). Members have a role of `editor` or `traveler` and a status of `invited` (not yet accepted) or `joined`. Declining, leaving and removal delete the membership row. Invited people have no access until they accept.

| Role | Can do |
| ---- | ------ |
| owner | everything: edit, invite editors and travelers, change roles, remove members, manage and revoke invite links, change `editors_can_invite` and `members_can_post`, delete the trip |
| editor | edit the itinerary and journey fields; invite travelers (only if `editors_can_invite` is on); leave |
| traveler | read the members list; leave |
| invited / outsider | nothing beyond the read they are allowed by [Privacy](#privacy) (`404` if the trip is hidden from them, `403` on member endpoints if they can see it) |

Only the owner can invite someone as `editor`. Journeys without an owner (created before login existed) are read-only: no invites, no edits.

All endpoints are under `/api/v1` and require `Authorization: Bearer <token>`.

| Method + path | Body | Success |
| ------------- | ---- | ------- |
| `GET /journeys/:slug/members` | – | `200` list of members (members only, includes invited) |
| `POST /journeys/:slug/members` | `{"username","role"}` | `201` member (status `invited`) |
| `PATCH /journeys/:slug/members/:username` | `{"role"}` | `200 {}` (owner only) |
| `DELETE /journeys/:slug/members/:username` | – | `200 {}` (owner only; also cancels an invite) |
| `POST /journeys/:slug/leave` | – | `200 {}` (the owner cannot leave) |
| `GET /journeys/:slug/invite-links` | – | `200` list of active links (owner, or editor when allowed) |
| `POST /journeys/:slug/invite-links` | `{"role"}` | `201 {"url","link"}`; the link expires after 7 days and the URL is `{APP_WEB_URL}/join/{token}` |
| `DELETE /journeys/:slug/invite-links/:id` | – | `200 {}` (owner only) |
| `POST /invites/join` | `{"token"}` | `200 {"slug"}`; the token goes in the body so it stays out of access logs |
| `GET /me/invites` | – | `200` pending invitations for the current user |
| `POST /me/invites/:id/accept` | – | `200 {}` |
| `POST /me/invites/:id/decline` | – | `200 {}` |
| `GET /me/journeys` | – | `200` trips the user owns or has joined: `slug, title, destination, departure_date, return_date, cover_url, role`, newest `departure_date` first |

Errors: `403` not allowed (the trip is visible to you but you may not do this), `404` unknown or hidden trip / member / invite, and `404 user not found` when the username on invite is empty or unknown, `409` already a member or trip full, `400` invalid role.

---

## Itinerary editing

Owner and editors can change a trip piece by piece; travelers and other viewers who can see the trip get `403`; a trip hidden from the caller is `404`. All endpoints require login. Absent JSON fields are left unchanged.

| Method + path | Body | Success |
| ------------- | ---- | ------- |
| `PATCH /journeys/:slug` | any of `title, destination, country, cover_url, departure_date, return_date, editors_can_invite, members_can_post, visibility, is_draft, timezone, hide_live_status` (`editors_can_invite`, `members_can_post`, `visibility`, `hide_live_status`: owner only) | `200` journey |
| `POST /journeys/:slug/days` | `{"date","date_iso","title"}` (`date`, `date_iso` required) | `201` day |
| `PATCH /journeys/:slug/days/:dayId` | any of `date, date_iso, title` | `200` day |
| `DELETE /journeys/:slug/days/:dayId` | – | `200 {}` (deletes its plans); `409 day has moments` if the day or any of its plans has [moments](#moments) |
| `POST /journeys/:slug/days/:dayId/plans` | `{"description","time","emoji","map_url"}` (`description` required) | `201` plan, appended last |
| `PUT /journeys/:slug/days/:dayId/plans/order` | `{"plan_ids":[...]}` (exactly the day's plans) | `200 {}` |
| `PATCH /journeys/:slug/plans/:planId` | any of `time, description, emoji, map_url, category, hidden_from_public, day_id` (`day_id` moves it to another day of the same trip, last position) | `200` plan |
| `DELETE /journeys/:slug/plans/:planId` | – | `200 {}` |

Rules: only the owner may change `editors_can_invite` and `members_can_post`; `cover_url` must be `https://` with a host and at most 2048 bytes; `map_url` must be an `http`/`https` URL with a host and at most 2048 bytes (a scheme-less value such as `maps.app.goo.gl/x` gets `https://` prefixed; other schemes are rejected, and errors on a full create/PUT name the position, e.g. `day 2, plan 3: invalid map_url`); `return_date` may not be before `departure_date`. A day or plan that belongs to another trip is `404` even if its ID exists. `PUT /journeys/:slug/update` is owner-only and replaces all days and plans with fresh IDs; `cover_url`, `editors_can_invite` and `members_can_post` are PATCH-only. Invite links stop working when their creator can no longer invite (removed, demoted, or `editors_can_invite` turned off).

---

## Privacy

Every read of a trip goes through one set of rules. The viewer's relation to a trip is, strongest first:

- **member** — the owner, or an editor/traveler who has **joined** (an invite not yet accepted gives nothing)
- **follower** — follows the trip's owner
- **stranger** — anyone else, including requests without a token

`visibility` is `public`, `followers` (the default), `link` or `private`. It is set on create or with `PATCH /journeys/:slug` (owner only). Trips created before login existed (no owner) count as `public`.

| `visibility` | Open by slug (`GET /journeys/:slug`) | Shown in lists (`GET /journeys`, profiles) |
| ------------ | ------------------------------------ | ------------------------------------------ |
| `public` | everyone | everyone |
| `followers` | members and followers | members and followers |
| `link` | everyone who has the slug | members only |
| `private` | members only | members only |

A trip the viewer may not see answers **`404 journey not found`** on every route, reads and writes alike, so its existence is not revealed.

**What strangers see.** Until a trip is completed, a stranger gets a redacted view (`"redacted": true`): `slug, title, destination, country, cover_url, visibility, owner` and a `status` that is only ever `planning` (draft) or `upcoming` with all counters `0` — never `ongoing`. There are no dates, no `month_start`/`month_end`, no days, plans, members or `live`. Once the trip is completed, strangers see it like a follower does. Lists that hold redacted trips are ordered by creation time (newest first), never by trip dates, so neither the content nor the order of a list tells a stranger that someone is away right now.

**Hidden plans.** A plan with `hidden_from_public: true` is shown to members only. New plans with `category: "stay"` or emoji `🏨` are hidden automatically unless the client sends `hidden_from_public` itself.

**On the trip now.** While a trip is ongoing (counted in calendar days in the trip's `timezone`, default `Asia/Bangkok`), members get `live: {city, day_number, total_days, today_plans}` and followers get `live: {city, day_number, total_days}`. Strangers get nothing. With **`hide_live_status: true`** (owner only), followers get no `live` either and the status shows `upcoming` with counters `0`; they still see the dates.

---

## Profile and Follow

Following is one-way. You cannot follow yourself. By default a user **approves followers** (`follow_approval: true`): following them creates a pending request, and a pending requester is not a follower (they see what a stranger sees) until the user accepts. A user can turn it off with `PATCH /me {"follow_approval": false}`; pending requests are then accepted (except from block-related users) and later follows are instant.

| Method + path | Auth | Success |
| ------------- | ---- | ------- |
| `POST /users/:username/follow` | yes | `200 {"is_following", "requested", "followers"}`; `requested: true` when approval is on; repeating is fine; `429` after 200 outgoing pending requests |
| `DELETE /users/:username/follow` | yes | `200 {"is_following": false, "requested": false, "followers": n}`; also cancels a pending request |
| `GET /me/follow-requests` | yes | `200 {"items": [{"user", "created_at"}], "count": n}`, newest first; `count` excludes block-related users (items cap at 100) |
| `POST /me/follow-requests/:username/accept` | yes | `200 {"accepted": true}`; `404` if no such request (or block-related) |
| `DELETE /me/follow-requests/:username` | yes | `200 {"declined": true}`; idempotent; `404` if block-related |
| `GET /me/followers` | yes | `200 {"items": [{"user", "created_at"}]}`, newest first |
| `DELETE /me/followers/:username` | yes | `200 {"removed": true}`; idempotent; `404` if block-related |
| `GET /users/:username/profile` | optional | `200` profile page, as the viewer may see it |

Errors: `400 cannot follow yourself`, `401` without a token on follow/unfollow and on every `/me/follow-requests` and `/me/followers` route, `404 user not found` for an unknown or block-related username (decline, remove follower and unfollow still delete any leftover rows first), and `404` on accept when there is no such request.

The profile shows the trips the user owns or has **joined** (not pending invites), limited to what the viewer may see in lists (table above). Each trip is the same view `GET /journeys/:slug` returns for that viewer, without days except for `now` when the viewer is a member.

```
{
  "user": {"username", "display_name", "avatar_url", "bio"},
  "followers": 12, "following": 3,
  "is_following": true,
  "follow_requested": false,
  "follow_approval": true,
  "is_blocked": false,
  "now": { ...trip, "live": {...} },
  "upcoming": [ ...trips ],
  "calendar": [ {"slug", "title", "destination", "country", "departure_date", "return_date", "month_start", "month_end", "companions": [ ...users ]} ],
  "past": [ ...trips ],
  "stats": {"countries": 2, "cities": 3, "trips": 3}
}
```

- `is_following` — the viewer follows this user (`false` without a token).
- `follow_requested` — the viewer has a pending request to follow this user. `follow_approval` — this user approves new followers.
- `is_blocked` — the viewer has blocked this user. When `true` the page is empty (`now: null`, `upcoming`/`calendar`/`past` empty, stats 0) so the viewer can unblock. A user blocked by the profile owner gets `404` instead.
- `now` — a trip the viewer may see live (see [Privacy](#privacy)); `null` otherwise. It is not repeated in `upcoming`.
- `upcoming` — trips whose status for the viewer is `planning` or `upcoming` (for strangers this includes trips that are actually underway), nearest departure first; newest created first if any of them is redacted.
- `past` — completed trips, latest return first.
- `calendar` — every trip on the profile. Dates, months and `companions` (the joined members other than the profile user) only appear when the viewer's view of the trip is not redacted. Ordered by departure, or newest created first if any item is redacted.
- `stats` — distinct countries, distinct destinations (cities) and number of trips, counted from `past` only, so trips the viewer cannot see never count.

---

## Feed

`GET /feed?before=&limit=` (login required) lists events derived on read from trips the viewer may see: those of people they follow, plus trips they own or joined. Event types: `trip_planned`, `trip_started`, `trip_completed`, `moments_added` (one item per uploader, trip day and local date). Newest first; `limit` must be an integer (default 20, max 50, otherwise `400`); `next_before` is an opaque cursor to pass as `before` (malformed cursor is `400`). Item ids are stable and do not contain usernames.

Page shape: `{"items": [...], "next_before": "<opaque cursor>" | null}`. Item shape:

```
{
  "id": "trip_started:<trip id>", "type": "trip_started", "at": "2026-12-10T00:00:00+07:00",
  "actor": {"username", "display_name", "avatar_url", "bio"},
  "trip": {"slug", "title", "destination", "country", "cover_url", "departure_date"?, "return_date"?},
  "companions": [ ...users, at most 5 ],
  "moments": {"count": 3, "day_id": "...", "day_title": "...", "thumbs": ["url", ...]}   // moments_added only
}
```

`trip_started` / `trip_completed` are only emitted for trips with both dates set. A `moments_added` group's `at` is the time of its newest moment, so it can move forward (and the item reappears near the top on refresh) as new moments arrive. Users who are block-related to the viewer never appear as actor or companion.

`GET /feed/now` (login required) returns `{"travelers": [{"user": {...}, "trip": {...same trip shape...}, "day_number": 2, "total_days": 5}]}`: followed people currently on a trip the viewer may see as live, one per person.

## Block

| Method + path | Auth | Success |
| ------------- | ---- | ------- |
| `POST /users/:username/block` | yes | `204`; blocking twice is fine |
| `DELETE /users/:username/block` | yes | `204`; unblocking someone not blocked is fine |
| `GET /me/blocks` | yes | `200 {"users": [...]}` |

Blocking yourself is `400`. Blocking removes follows in both directions. Block-related users cannot follow each other (`404 user not found`), a blocked user gets `404` on the blocker's profile endpoints (`GET /users/:username`, `/users/:username/profile`), and each side's trips, moments and feed entries are hidden from the other (existing trip members keep access).

## Report

`POST /reports` (login required) with `{"target_type": "user|journey|moment", "target": "<username|slug|moment id>", "reason": "spam|harassment|inappropriate|privacy|other", "note": "optional, max 500 chars"}`.

- `201` for a new report, `200` if you already have an open report on the same target.
- Response data is `{"id","target_type","reason","status","created_at"}`; the target id is never returned.
- Targets that are missing or not visible to you are `404`; invalid reason/type is `400`; more than 20 reports in 24 hours is `429`.

Moderation CLI (run on the server; log output goes to stderr, results to stdout):

```
./server reports              # list open reports
./server resolve-report <id>  # mark a report resolved
./server delete-moment <id>   # delete a reported moment
```

---

## Moments

Photos (jpeg, png, webp, up to 10 MB) and short videos (mp4, mov, up to 100 MB and 60 s) attached to a trip day or plan. Files never pass through the API on upload: the client asks for a signed URL, uploads straight to storage, then completes the moment. The server strips metadata (EXIF/GPS), re-encodes and makes a thumbnail; raw files are never served. Signed upload URLs last 15 minutes and view URLs 30 to 90 minutes (they expire on the hour and are identical within an hour, so clients can cache them). Uploads left pending for over an hour are cleaned up every 30 minutes.

| Endpoint | Auth | Description |
| -------- | ---- | ----------- |
| `POST /journeys/:slug/moments/uploads` | yes | body `{kind, content_type, size_bytes, day_id, plan_id?, caption?}` returns `201 {moment, upload{url,method,headers,expires_at}}`; `PUT` the file to `upload.url` with `upload.headers` verbatim |
| `POST /journeys/:slug/moments/:id/complete` | yes | processes the upload, returns `200` moment |
| `GET /journeys/:slug/moments` | optional | `200 {"moments": [...]}`, limited by [Privacy](#privacy) |
| `PATCH /moments/:id` | yes | body `{"caption"}`, uploader only |
| `DELETE /moments/:id` | yes | uploader or trip owner, `204` |
| `GET /me/moments` | yes | your moments across trips |

Errors: `404` for unknown or hidden trips and moments, `409` when deleting a day that has moments (`day has moments`), when a full `PUT /journeys/:slug/update` targets a trip that has moments (`trip has moments`), or when a trip reaches its moment limit. Deleting a trip deletes its moments and files. Video needs `ffmpeg`/`ffprobe` (included in the Docker image); without them video moments are rejected with `video processing is unavailable`.

### Moments / Firebase Storage

1. In the Firebase console enable **Storage** (requires the Blaze plan) and note the bucket name.
2. Create a service account with the role **Storage Object Admin** on that bucket and download its JSON key.
3. Set `STORAGE_DRIVER=firebase`, `FIREBASE_STORAGE_BUCKET` and `FIREBASE_SERVICE_ACCOUNT_JSON` (raw JSON or base64) on the service.
4. Set the bucket CORS so browsers can `PUT` and `GET` through signed URLs. Save as `cors.json` (use your real web origin) and apply it:

```json
[
  {
    "origin": ["https://your-web-app.example.com", "http://localhost:3000"],
    "method": ["PUT", "GET"],
    "responseHeader": ["Content-Type", "x-goog-content-length-range"],
    "maxAgeSeconds": 3600
  }
]
```

```bash
gcloud storage buckets update gs://<bucket> --cors-file=cors.json
```

5. Set the Firebase Storage rules to deny everything, since all access goes through signed URLs: `allow read, write: if false;`.
6. Add a lifecycle rule that deletes `uploads/` objects older than 1 day. Raw uploads are normally deleted when a moment is completed or cleaned up, but a client that uploads and never completes (or a failed cleanup) would otherwise leave orphaned raw files behind. Save as `lifecycle.json` and apply it:

```json
{"rule":[{"action":{"type":"Delete"},"condition":{"age":1,"matchesPrefix":["uploads/"]}}]}
```

```bash
gcloud storage buckets update gs://<bucket> --lifecycle-file=lifecycle.json
```

For local development `STORAGE_DRIVER=local` (the default when `APP_ENV` is `development` or `test`) stores files under `./.data/media` and serves them from `/api/v1/media/local/*`. It is refused in any other `APP_ENV`.

---

## Response Format

### Success

```json
{
  "code": "JOURNEY-200000",
  "message": "Success",
  "data": {}
}
```

### Error

```json
{
  "code": "JOURNEY-400000",
  "message": "title and destination are required",
  "error": {}
}
```

### Error Codes

| HTTP Status | Code             |
| ----------- | ---------------- |
| 200         | `JOURNEY-200000` |
| 201         | `JOURNEY-201000` |
| 400         | `JOURNEY-400000` |
| 401         | `JOURNEY-401000` |
| 403         | `JOURNEY-403000` |
| 404         | `JOURNEY-404000` |
| 500         | `JOURNEY-500000` |

---

## Project Structure

```
journey-service/
├── main.go
├── config/
│   └── config.go                  # loads env variables
├── database/
│   └── database.go                # connects to DB and runs migrations
├── internal/
│   ├── handlers/
│   │   └── journey_handler.go     # request handling
│   ├── middleware/
│   │   └── middleware.go          # recovery, request ID, CORS, logger
│   ├── models/
│   │   └── journey.go             # Journey, ItineraryDay, Plan structs
│   ├── repository/
│   │   ├── journey_repository.go  # interface
│   │   └── journey_gorm.go        # database queries
│   ├── routes/
│   │   └── routes.go              # route definitions
│   └── services/
│       └── journey_service.go     # business logic
└── pkg/
    ├── logger/
    │   └── logger.go              # zerolog (pretty in dev, JSON in prod)
    └── utils/
        └── response.go            # success/error response helpers
```

---

## Configuration

Copy `.env.example` and fill in your values:

```bash
cp .env.example .env
```

| Variable   | Default              | Description                                           |
| ---------- | -------------------- | ----------------------------------------------------- |
| `APP_PORT` | `8080`               | port the server listens on                            |
| `APP_ENV`  | `development`        | `development` = pretty logs, `production` = JSON logs |
| `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DBNAME`, `POSTGRES_SSLMODE` | | PostgreSQL connection |
| `APP_WEB_URL` | | web app base URL, used in email login links |
| `GOOGLE_CLIENT_ID` | | Google OAuth client ID used to verify Google ID tokens |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM` | | SMTP settings for sending login emails; port `465` uses implicit TLS, other ports use STARTTLS; the server refuses to send without TLS unless `SMTP_HOST` is a loopback address; each send gives up after 15 s |
| `APP_CLIENT_IP_HEADER` | `X-Real-IP` | header holding the caller's IP behind the proxy (Railway sets `X-Real-IP`); used only for the per-IP login-email limit, falls back to the connection address when missing or not an IP |
| `STORAGE_DRIVER` | `local` in development/test | `firebase` or `local`; must be `firebase` in any other `APP_ENV` (the server refuses to start otherwise) |
| `FIREBASE_STORAGE_BUCKET`, `FIREBASE_SERVICE_ACCOUNT_JSON` | | bucket name and service-account key (raw JSON or base64) for `firebase` |
| `LOCAL_STORAGE_DIR` | `./.data/media` | where the `local` driver keeps files |
| `APP_PUBLIC_API_URL` | `http://localhost:<APP_PORT>/api/v1` | public API base URL, used to build `local` upload/view URLs |

### Production

Set these on the deployed service: `APP_ENV=production`, `STORAGE_DRIVER=firebase` with `FIREBASE_STORAGE_BUCKET` and `FIREBASE_SERVICE_ACCOUNT_JSON` (see [Moments / Firebase Storage](#moments--firebase-storage)), `APP_WEB_URL` (absolute http(s) URL), `GOOGLE_CLIENT_ID`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, and `POSTGRES_*`. The service refuses to start unless `APP_ENV` is `development` or `test` when `SMTP_HOST` is empty (the log mailer prints login links), and refuses to start with SMTP if `APP_WEB_URL` is not an absolute http(s) URL.

Login emails are limited three ways: 5 per address per hour, 20 per client IP per hour (in memory, resets on restart; IPv6 counted per /64; without the IP header the first `X-Forwarded-For` hop is used, and a warning is logged if neither is present) and 200 in total per hour; past any of them `POST /auth/email/start` returns `429`. The total cap protects the SMTP account if the IP header is spoofed or shared. Expired sessions and login tokens older than a day are deleted at startup and every 6 hours.

The service runs on **Railway** (Docker image from the `Dockerfile`, which includes `ffmpeg`/`ffprobe`). Set the variables above once in the Railway service's Variables tab; Railway keeps them across deploys. Before the first deploy of the Moments release, make sure at least these are set:

| Variable | Value |
| -------- | ----- |
| `APP_ENV` | `production` |
| `STORAGE_DRIVER` | `firebase` |
| `FIREBASE_STORAGE_BUCKET` | your bucket name |
| `FIREBASE_SERVICE_ACCOUNT_JSON` | the service-account key, base64-encoded |
| `APP_WEB_URL`, `GOOGLE_CLIENT_ID`, `SMTP_*`, `POSTGRES_*` | as above |

Use base64 for `FIREBASE_SERVICE_ACCOUNT_JSON` on Railway: the key's `private_key` contains newlines and quotes that are easy to mangle in a variables editor. Encode it with `base64 < key.json | tr -d '\n'` (the server accepts raw JSON or base64).

**Alternative: Cloud Run.** Set the same variables on the Cloud Run service (console, or `gcloud run services update journey-service --update-env-vars ...`; prefer Secret Manager for `POSTGRES_PASSWORD`, `SMTP_PASSWORD` and the service-account key). `make deploy.run` uses `--update-env-vars`, so it keeps them on every deploy.

**Breaking change:** creating, updating and deleting journeys now requires `Authorization: Bearer <token>`. Existing journeys created before login existed have no owner and are read-only.

**Breaking change for deploy:** outside `APP_ENV=development`/`test` the server now refuses to start unless `STORAGE_DRIVER=firebase` and both `FIREBASE_STORAGE_BUCKET` and `FIREBASE_SERVICE_ACCOUNT_JSON` are set (and the credentials are usable). Set the Railway variables listed above before deploying this release, or the new deployment will crash on boot.

### Claiming existing trips

Trips created before login existed have no owner. To claim them: log in once to create your account (optionally set your username via `PATCH /api/v1/me`), then give all ownerless trips to that username. The command also makes those trips `public` and hides their hotel plans (emoji `🏨` or category `stay`) from non-members. It logs how many trips were claimed. Soft-deleted trips are left alone and the command is safe to re-run (it then claims 0). It needs only the database settings, not SMTP or `APP_WEB_URL`.

```bash
# locally, with the production database env loaded (.env or exported POSTGRES_*)
go run . assign-legacy-owner <username>

# production runs on Railway: recommended, in the Railway service shell
# (the Dockerfile builds the binary as /app/server)
./server assign-legacy-owner <username>

# note: `railway run` executes the command on your machine, so it may not reach a private
# *.railway.internal database host; only use it if the database is reachable from your machine
```

---

## Getting Started

**Requirements:** Go 1.24+, PostgreSQL

```bash
# Install dependencies
go mod download

# Run locally (loads .env automatically)
make run

# The server starts at http://localhost:8080
# Interactive API docs: http://localhost:8080/docs
# Prometheus metrics:   http://localhost:8080/metrics
```

### Make Commands

| Command         | Description                                         |
| --------------- | --------------------------------------------------- |
| `make run`      | run the server                                      |
| `make build`    | build binary to `bin/journey-service` (linux/amd64) |
| `make test`     | run tests and open coverage report                  |
| `make ci.lint`  | run linter with auto-fix                            |
| `make swag.gen` | generate Swagger docs                               |
| `make mock.gen` | generate mocks                                      |
| `make wire`     | run wire dependency injection                       |
