# Announcement Email Broadcast

Sub2API administrators can optionally broadcast an announcement to every eligible user email address when the announcement is active. The announcement is saved together with a durable email job, while SMTP delivery runs in the background.

## Delivery semantics

- The option is disabled by default for every create/edit session.
- Email delivery is available only when the final announcement status is `active`.
- A future `starts_at` delays the job until that time.
- Each announcement can create at most one broadcast job. Editing or submitting it again does not create another broadcast.
- The job stores a title/content snapshot. Later announcement edits do not change or resend the queued email.
- Email targeting is intentionally independent from announcement display `targeting`: the broadcast covers all eligible users.
- The broadcast uses the user's primary email only. Extra balance-notification addresses are not included.
- This is a transactional system-announcement event and cannot be unsubscribed.

An eligible recipient is an active, non-deleted user whose primary email has a matching verified `email` AuthIdentity. Reserved/synthetic addresses and users without a verified primary email identity are excluded.

## Architecture

The HTTP request never loops over users or waits for all SMTP deliveries. Publishing with email notification performs one database transaction that:

1. saves the announcement; and
2. creates a unique `announcement_email_jobs` record containing the announcement snapshot.

A background runtime then:

1. claims jobs with a database lease;
2. materializes recipients with a stable user-ID keyset cursor;
3. creates one unique `announcement_email_deliveries` row per user;
4. claims due deliveries with bounded concurrency;
5. renders the `announcement.published` notification template; and
6. records success, retry, failure, ambiguous delivery, and aggregate job counts.

Database leases and unique constraints allow multiple application instances and process restarts without relying on the in-memory verification-code email queue.

## Configuration

```yaml
announcement_email:
  enabled: true
  recipient_batch_size: 500
  delivery_worker_count: 4
  delivery_batch_size: 50
  poll_interval_seconds: 5
  lease_seconds: 120
  max_attempts: 5
  retry_base_seconds: 30
  max_retry_seconds: 3600
  send_timeout_seconds: 30
```

The application clamps invalid or unsafe values to bounded defaults. SMTP host, credentials, sender, and TLS mode continue to use the existing settings under **Admin → Settings → Email**.

When `announcement_email.enabled` is false, normal announcement publishing remains available, but requests with `send_email=true` are rejected.

## Notification template

The existing email-template editor exposes a new event:

```text
announcement.published
```

Supported placeholders:

- `{{site_name}}`
- `{{recipient_name}}`
- `{{recipient_email}}`
- `{{announcement_title}}`
- `{{announcement_content}}`
- `{{announcement_starts_at}}`

English and Chinese official templates are included. Announcement title and content are HTML-escaped before rendering; administrator-provided Markdown or HTML is never injected as trusted raw HTML.

## Admin API

All endpoints require the existing administrator authentication and compliance guard.

### Check capability and recipient estimate

```http
GET /api/v1/admin/announcements/email-capability
```

Response data:

```json
{
  "enabled": true,
  "smtp_configured": true,
  "eligible_count": 1248
}
```

The count is an estimate at request time. A scheduled job determines its stable recipient cutoff when it starts preparing recipients.

### Create and broadcast an announcement

```http
POST /api/v1/admin/announcements
Content-Type: application/json
Idempotency-Key: 2f38d38a-8939-47c6-84da-f88266dc0d2d
```

```json
{
  "title": "Planned maintenance",
  "content": "Maintenance begins at 02:00 UTC.",
  "status": "active",
  "notify_mode": "popup",
  "targeting": { "any_of": [] },
  "send_email": true
}
```

`Idempotency-Key` is required when `send_email=true`. Reusing the same key with the same administrator, route, method, and request body replays the original result instead of creating another announcement.

The returned announcement contains a lightweight summary when a job exists:

```json
{
  "id": 42,
  "title": "Planned maintenance",
  "status": "active",
  "email_notification": {
    "status": "pending",
    "total_count": 0,
    "sent_count": 0,
    "failed_count": 0,
    "ambiguous_count": 0,
    "skipped_count": 0,
    "available_at": "2026-07-01T02:00:00Z",
    "can_retry": false
  }
}
```

The same `send_email` field is accepted by:

```http
PUT /api/v1/admin/announcements/:id
```

An already active announcement without a job can create its one broadcast job. If a job already exists, another broadcast is not created.

### Get detailed email status

```http
GET /api/v1/admin/announcements/:id/email-notification
```

The response includes status, aggregate counts, scheduling/start/finish timestamps, a sanitized error summary, and retry capability. Recipient addresses are not returned by this endpoint.

Job statuses:

- `pending`: waiting for `available_at`.
- `preparing`: materializing the stable recipient set.
- `sending`: deliveries are being processed or retried.
- `completed`: all terminal deliveries completed without failure.
- `completed_with_failures`: one or more deliveries failed or are ambiguous.
- `failed`: the job could not progress, for example because of invalid SMTP configuration.
- `cancelled`: the announcement was no longer eligible before delivery began.

### Retry failed delivery

```http
POST /api/v1/admin/announcements/:id/email-notification/retry
Content-Type: application/json
Idempotency-Key: 489ae87d-0434-4bc0-b690-fc00ce35fd46
```

```json
{
  "include_ambiguous": false
}
```

By default, only deterministically failed deliveries are reset. `include_ambiguous=true` also retries deliveries whose SMTP result was uncertain and may therefore produce a duplicate email; administrator confirmation is required in the dashboard before using it.

Use a new idempotency key for each intentional retry command, and reuse that key only when retrying the same HTTP request after a network failure.

## Retry and delivery guarantees

Temporary failures that are known to occur before SMTP acceptance are retried with exponential backoff up to `max_attempts`. Configuration/authentication failures are surfaced without a tight retry loop.

SMTP cannot provide strict exactly-once delivery. A connection can fail after the message body is submitted but before the server response is observed. Sub2API records this as `ambiguous` and does not retry it automatically. Database uniqueness, conditional state changes, and leases prevent ordinary duplicate work, while ambiguous retries remain an explicit administrator decision.

## Operational checks

- Confirm SMTP settings with the existing test-email action before enabling a large broadcast.
- Keep `delivery_worker_count` conservative for providers with strict connection or rate limits.
- Monitor `failed_count`, `ambiguous_count`, and `last_error_code` in the announcement email status dialog.
- If SMTP configuration is corrected after a job-level failure, use the retry action instead of editing or republishing the announcement.
- Archiving a scheduled announcement before the job starts cancels it. Emails already accepted by SMTP cannot be recalled.
