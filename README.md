# journey-service

## Table of Contents

- [Overview](#overview)
- [How It Works](#how-it-works)
- [Auth](#auth)
- [API Reference](#api-reference)
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
| PATCH  | `/me`                   | yes  | update `username`, `display_name`, `bio`, `avatar_url` (https, max 2048 bytes) |
| GET    | `/api/v1/users/:username`      | no   | public profile (no email)                                 |

Journeys have an owner (`owner_id`, set from the logged-in user on create). Creating, updating and deleting a journey requires login; only the owner may update or delete it (`403 forbidden` otherwise). Reading journeys is public. Journeys created before login existed have an empty `owner_id` and are read-only; to hand one to a user, set its `owner_id` in the database by hand.

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

Returns a single journey with all days and plans.

**Response:** `200 OK`

---

### Update a Journey

```
PUT /journeys/:slug/update
```

Requires login; owner only.

Replaces the journey including all days and plans. Make sure to include every day and plan you want to keep — anything not sent will be deleted.

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
| `DB_DSN`   | `host=localhost ...` | PostgreSQL connection string                          |
| `APP_WEB_URL` | | web app base URL, used in email login links |
| `GOOGLE_CLIENT_ID` | | Google OAuth client ID used to verify Google ID tokens |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM` | | SMTP settings for sending login emails |

### Production

Set these on the deployed service: `APP_ENV=production`, `APP_WEB_URL` (absolute http(s) URL), `GOOGLE_CLIENT_ID`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, and `POSTGRES_*`. The service refuses to start unless `APP_ENV` is `development` or `test` when `SMTP_HOST` is empty (the log mailer prints login links), and refuses to start with SMTP if `APP_WEB_URL` is not an absolute http(s) URL.

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
