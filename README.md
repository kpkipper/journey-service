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
| PATCH  | `/api/v1/me`                   | yes  | update `username`, `display_name`, `bio`, `avatar_url` (https, max 2048 bytes) |
| GET    | `/api/v1/users/:username`      | no   | public profile (no email)                                 |

Journeys have an owner (`owner_id`, set from the logged-in user on create). Creating, updating and deleting a journey requires login; only the owner may update or delete it (`403 forbidden` otherwise). Who may read a journey, and how much of it, follows the [Privacy](#privacy) rules. Journeys created before login existed have an empty `owner_id` and are read-only; to hand one to a user, set its `owner_id` in the database by hand.

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

Returns journeys grouped by country. Journeys without a country are grouped under `"Other"`.

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
          "title": "Japan Trip 2026",
          "destination": "Tokyo, Japan"
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

Returns a single journey with all days and plans, plus `members`: the owner and every joined member (`username`, `display_name`, `avatar_url`, `role`, `status`). Pending invites are not listed. Login is optional; what the response contains depends on the viewer (see [Privacy](#privacy)).

**Response:** `200 OK`

---

### Update a Journey

```
PUT /journeys/:slug/update
```

Requires login; **owner only** (`403` for editors and everyone else).

Updates only `title`, `destination`, `country`, `departure_date` and `return_date`, and replaces all days and plans. `cover_url` and `editors_can_invite` are not touched by this endpoint: change them with `PATCH /journeys/:slug`. A deleted trip is `404`. Make sure to include every day and plan you want to keep — anything not sent will be deleted. Days and plans are re-created with fresh IDs on every call, so IDs from an earlier `GET` are stale afterwards. Prefer the fine-grained endpoints under [Itinerary editing](#itinerary-editing) for small changes.

**Request body:** same as Create

**Response:** `200 OK`

---

### Delete a Journey

```
DELETE /journeys/:slug
```

Requires login; owner only. Returns `404` if the journey does not exist.

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
| owner | everything: edit, invite editors and travelers, change roles, remove members, manage and revoke invite links, change `editors_can_invite`, delete the trip |
| editor | edit the itinerary and journey fields; invite travelers (only if `editors_can_invite` is on); leave |
| traveler | read the members list; leave |
| invited / outsider | nothing beyond the public read (`403` on member endpoints) |

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

Errors: `403` not allowed, `404` unknown trip / member / invite, and `404 user not found` when the username on invite is empty or unknown, `409` already a member or trip full, `400` invalid role.

---

## Itinerary editing

Owner and editors can change a trip piece by piece; travelers and non-members get `403`. All endpoints require login. Absent JSON fields are left unchanged.

| Method + path | Body | Success |
| ------------- | ---- | ------- |
| `PATCH /journeys/:slug` | any of `title, destination, country, cover_url, departure_date, return_date, editors_can_invite, visibility, is_draft, timezone, hide_live_status` (`editors_can_invite`, `visibility`, `hide_live_status`: owner only) | `200` journey |
| `POST /journeys/:slug/days` | `{"date","date_iso","title"}` (`date`, `date_iso` required) | `201` day |
| `PATCH /journeys/:slug/days/:dayId` | any of `date, date_iso, title` | `200` day |
| `DELETE /journeys/:slug/days/:dayId` | – | `200 {}` (deletes its plans) |
| `POST /journeys/:slug/days/:dayId/plans` | `{"description","time","emoji","map_url"}` (`description` required) | `201` plan, appended last |
| `PUT /journeys/:slug/days/:dayId/plans/order` | `{"plan_ids":[...]}` (exactly the day's plans) | `200 {}` |
| `PATCH /journeys/:slug/plans/:planId` | any of `time, description, emoji, map_url, day_id` (`day_id` moves it to another day of the same trip, last position) | `200` plan |
| `DELETE /journeys/:slug/plans/:planId` | – | `200 {}` |

Rules: only the owner may change `editors_can_invite`; `cover_url` must be `https://` with a host and at most 2048 bytes; `map_url` must be an `http`/`https` URL with a host and at most 2048 bytes (a scheme-less value such as `maps.app.goo.gl/x` gets `https://` prefixed; other schemes are rejected, and errors on a full create/PUT name the position, e.g. `day 2, plan 3: invalid map_url`); `return_date` may not be before `departure_date`. A day or plan that belongs to another trip is `404` even if its ID exists. `PUT /journeys/:slug/update` is owner-only and replaces all days and plans with fresh IDs; `cover_url` and `editors_can_invite` are PATCH-only. Invite links stop working when their creator can no longer invite (removed, demoted, or `editors_can_invite` turned off).

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

Following is instant (no approval) and one-way. You cannot follow yourself.

| Method + path | Auth | Success |
| ------------- | ---- | ------- |
| `POST /users/:username/follow` | yes | `200 {"is_following": true, "followers": n}`; following twice is fine |
| `DELETE /users/:username/follow` | yes | `200 {"is_following": false, "followers": n}`; unfollowing twice is fine |
| `GET /users/:username/profile` | optional | `200` profile page, as the viewer may see it |

Errors: `400 cannot follow yourself`, `401` without a token on follow/unfollow, `404 user not found` for an unknown username.

The profile shows the trips the user owns or has **joined** (not pending invites), limited to what the viewer may see in lists (table above). Each trip is the same view `GET /journeys/:slug` returns for that viewer, without days except for `now` when the viewer is a member.

```
{
  "user": {"username", "display_name", "avatar_url", "bio"},
  "followers": 12, "following": 3,
  "is_following": true,
  "now": { ...trip, "live": {...} },
  "upcoming": [ ...trips ],
  "calendar": [ {"slug", "title", "destination", "country", "departure_date", "return_date", "month_start", "month_end", "companions": [ ...users ]} ],
  "past": [ ...trips ],
  "stats": {"countries": 2, "cities": 3, "trips": 3}
}
```

- `is_following` — the viewer follows this user (`false` without a token).
- `now` — a trip the viewer may see live (see [Privacy](#privacy)); `null` otherwise. It is not repeated in `upcoming`.
- `upcoming` — trips whose status for the viewer is `planning` or `upcoming` (for strangers this includes trips that are actually underway), nearest departure first; newest created first if any of them is redacted.
- `past` — completed trips, latest return first.
- `calendar` — every trip on the profile. Dates, months and `companions` (the joined members other than the profile user) only appear when the viewer's view of the trip is not redacted. Ordered by departure, or newest created first if any item is redacted.
- `stats` — distinct countries, distinct destinations (cities) and number of trips, counted from `past` only, so trips the viewer cannot see never count.

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
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM` | | SMTP settings for sending login emails |

### Production

Set these on the deployed service: `APP_ENV=production`, `APP_WEB_URL` (absolute http(s) URL), `GOOGLE_CLIENT_ID`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, and `POSTGRES_*`. The service refuses to start unless `APP_ENV` is `development` or `test` when `SMTP_HOST` is empty (the log mailer prints login links), and refuses to start with SMTP if `APP_WEB_URL` is not an absolute http(s) URL.

Set these once on the Cloud Run service (console, or `gcloud run services update journey-service --update-env-vars ...`; prefer Secret Manager for `POSTGRES_PASSWORD` and `SMTP_PASSWORD`). `make deploy.run` uses `--update-env-vars`, so it keeps them on every deploy.

**Breaking change:** creating, updating and deleting journeys now requires `Authorization: Bearer <token>`. Existing journeys created before login existed have no owner and are read-only.

### Claiming existing trips

Trips created before login existed have no owner. To claim them: log in once to create your account (optionally set your username via `PATCH /api/v1/me`), then give all ownerless trips to that username. Soft-deleted trips are left alone and the command is safe to re-run. It needs only the database settings, not SMTP or `APP_WEB_URL`.

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
