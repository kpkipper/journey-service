# journey-service

## Table of Contents

- [Overview](#overview)
- [How It Works](#how-it-works)
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

## API Reference

Base URL: `/api/v1`

---

### Create a Journey

```
POST /journeys
```

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
GET /journeys/:id
```

Returns a single journey with all days and plans.

**Response:** `200 OK`

---

### Update a Journey

```
PUT /journeys/:id
```

Replaces the journey including all days and plans. Make sure to include every day and plan you want to keep — anything not sent will be deleted.

**Request body:** same as Create

**Response:** `200 OK`

---

### Delete a Journey

```
DELETE /journeys/:id
```

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
