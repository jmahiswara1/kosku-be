### Task 10 Report: Add email column to profiles and fix email notifications

**Status:** Complete

**Changes Made:**

#### Migration Files
- Created `db/migrations/000023_add_email_to_profiles.up.sql` — adds `email TEXT` column and index to profiles table
- Created `db/migrations/000023_add_email_to_profiles.down.sql` — drops index and column

#### Database Queries (sqlc)
- Updated `db/queries/profiles.sql` — added `email` to GetProfile, UpsertProfile, and UpdateProfile queries
- Updated `db/queries/announcements.sql` — added `p.email` to GetActiveTenantsByProperty and GetActiveTenantsByOwner queries

#### Generated Repository Code (manual updates since sqlc not available)
- Updated `internal/repository/models.go` — added `Email sql.NullString` to Profile struct
- Updated `internal/repository/profiles.sql.go` — updated all three query functions (GetProfile, UpsertProfile, UpdateProfile) to include email in SQL, params, and Scan
- Updated `internal/repository/announcements.sql.go` — updated GetActiveTenantsByOwnerRow and GetActiveTenantsByPropertyRow structs and Scan calls to include email

#### Auth Middleware
- Updated `internal/middleware/auth.go` — added `Email` field to `supabaseClaims` struct and `ContextKeyEmail` constant; middleware now stores email from JWT in gin context

#### DTOs
- Updated `internal/dto/auth.go` — added `Email` field to both `RegisterRequest` and `ProfileResponse`

#### Auth Service
- Updated `internal/service/auth.go`:
  - Register: extracts email from request, creates `sql.NullString`, passes to UpsertProfile
  - Approve: uses profile email as fallback when request email is empty
  - Reject: uses profile email as fallback when request email is empty
  - profileToDTO: includes email in response

#### Auth Handler
- Updated `internal/handler/auth.go` — Register handler extracts email from gin context (set by middleware) and sets it on the request DTO

#### Billing Service
- Updated `internal/service/billing.go`:
  - ConfirmPayment: reads tenant email from profile, skips email if not available
  - RejectPayment: reads tenant email from profile, skips email if not available

#### Scheduler Service
- Updated `internal/service/scheduler.go` — sendReminderForContract now uses profile email instead of placeholder addresses; skips sending if email not available

#### Ticket Service
- Updated `internal/service/ticket.go`:
  - CreateTicket: uses owner profile email instead of empty string
  - UpdateTicket: uses tenant profile email instead of empty string

#### Announcement Service
- Updated `internal/service/announcement.go` — tenantInfo struct now includes email; email sending uses actual profile email instead of empty string

**All previously empty email string calls (`""`) have been replaced with actual profile email lookups with proper null-checking.**
