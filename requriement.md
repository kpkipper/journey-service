1. init service สำหรับ CRUD (วางแผนการท่องเที่ยว)
2. can create plan
3. edit/update plan
4. get data to view plan

เช่น plan เที่ยว osaka ใน japan

- วันไป - กลับ
- แต่ละวันทำอะไรบ้าง

สิ่งที่ต้องเก็บตอน create plan ประมาณนี้ ItineraryDay[]
export type Plan = {
time: string
description: string
category?: string
emoji?: string
map?: string
}

export type ItineraryDay = {
date: string // '2 May (Sat)'
dateISO: string // UTC
title: string
plans: Plan[]
}

การวางโฟเดอร์กับไฟล์จะเป็นแบบ
Layered Architecture + Repository Pattern

HTTP Request
│
▼
┌─────────────────────────────┐
│ Middleware │ ← Recovery, RequestID, Auth, Logging, CORS, Pagination
├─────────────────────────────┤
│ Handlers (internal/handlers)│ ← รับ request, validate, ส่งต่อให้ service
├─────────────────────────────┤
│ Services (internal/services)│ ← Business logic ทั้งหมดอยู่ที่นี่
├─────────────────────────────┤
│ Repository (internal/repo) │ ← Data access ผ่าน interface, ใช้ GORM
├─────────────────────────────┤
│ Database (PostgreSQL) │ ← GORM ORM + Connection Pooling
└─────────────────────────────┘
lp-reward-store-api/
├── main.go # Entry point: init config → logger → database → fiber → graceful shutdown
├── config/ # โหลด config จาก env vars ผ่าน Viper + godotenv
├── database/ # Connection manager, health check, connection pooling
│
├── internal/ # ⚠️ Private code — ใช้ได้เฉพาะภายในโปรเจกต์
│ ├── constants/ # Error codes, coupon types, statuses, transitions
│ ├── models/ # GORM models (Coupon, UserCoupon, CouponHistory, UserCouponActivity)
│ ├── repository/ # Repository interfaces + implementations
│ ├── services/ # Business logic (CouponService, UserCouponService)
│ ├── handlers/ # HTTP handlers (Fiber)
│ ├── middleware/ # Middleware stack (auth, logging, error handler, pagination, etc.)
│ ├── routes/ # Route definitions — wiring ทุกอย่างเข้าด้วยกัน
│ └── clients/ # External clients (ECI API, Kafka producer)
│
├── pkg/ # 🌐 Public/reusable packages
│ ├── constants/ # Shared constants (context keys, etc.)
│ ├── context/ # Context helpers: trace_id, customer_id, session_id, user_id
│ ├── helpers/ # Validation, DB error handler, restrictions
│ ├── logger/ # Structured logging (Zerolog + Elasticsearch)
│ ├── pagination/ # Pagination utilities
│ ├── retryer/ # Retry logic
│ ├── utils/ # Error types + Response helpers
│ └── zlog/ # Zerolog wrapper
│
├── scripts/migration/ # Database migrations (SQL up/down files)
├── tests/integration/ # Integration tests
