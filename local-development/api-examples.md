# MiniMovie API Examples

Base URL: `http://localhost:8080`

---

## 1. Health Check

```http
GET http://localhost:8080/ping
```

Response: `200 OK`

```
pong
```

---

## 2. Public Catalog

All public catalog routes require the guest key header.

**Required header:**

```
Authorization: Bearer <guest-key>
```

### Search (multi)

```http
GET http://localhost:8080/search?q=fight+club
```

```json
{
  "page": 1,
  "totalPages": 1,
  "totalResults": 3,
  "results": [
    {
      "id": 550,
      "mediaType": "movie",
      "title": "Fight Club",
      "overview": "A ticking-Loss bomb insomniac...",
      "posterPath": "/pB8BM7pdSp6B6Ih7QI4S2t0POoF.jpg",
      "releaseDate": "1999-10-15"
    }
  ]
}
```

### Search by type

```http
GET http://localhost:8080/search?q=breaking+bad&type=series
```

```http
GET http://localhost:8080/search?q=brad+pitt&type=person&page=1
```

Valid `type` values: `movie`, `series`, `person`, `all` (or omit for multi-search).

### Get Movie

```http
GET http://localhost:8080/movies/550
```

```json
{
  "id": 550,
  "imdbID": "tt0137523",
  "title": "Fight Club",
  "tagline": "Mischief. Mayhem. Soap.",
  "overview": "A ticking-time bomb insomniac...",
  "genres": ["Drama"],
  "posterPath": "/pB8BM7pdSp6B6Ih7QI4S2t0POoF.jpg",
  "status": "Released",
  "releaseDate": "1999-10-15",
  "runtime": 139,
  "budget": 63000000,
  "revenue": 100853753,
  "voteAverage": 8.4,
  "credits": { "cast": [], "crew": [] },
  "collectionInfo": null
}
```

### Get Series

```http
GET http://localhost:8080/series/1396
```

```json
{
  "id": 1396,
  "name": "Breaking Bad",
  "tagline": "All Hail the King",
  "overview": "Walter White, a New Mexico chemistry teacher...",
  "genres": ["Drama", "Crime"],
  "status": "Ended",
  "firstAirDate": "2008-01-20",
  "lastAirDate": "2013-09-29",
  "numberOfSeasons": 5,
  "numberOfEpisodes": 62,
  "seasons": [
    {
      "id": 3572,
      "name": "Season 1",
      "seasonNumber": 1,
      "episodeCount": 7,
      "airDate": "2008-01-20"
    }
  ],
  "credits": { "cast": [], "crew": [] }
}
```

### Get Season

```http
GET http://localhost:8080/series/1396/seasons/1
```

```json
{
  "id": 3572,
  "name": "Season 1",
  "seasonNumber": 1,
  "airDate": "2008-01-20",
  "episodes": [
    {
      "id": 62085,
      "name": "Pilot",
      "episodeNumber": 1,
      "seasonNumber": 1,
      "airDate": "2008-01-20",
      "runtime": 58
    }
  ],
  "credits": { "cast": [], "crew": [] }
}
```

### Get Episode

```http
GET http://localhost:8080/series/1396/seasons/1/episodes/1
```

```json
{
  "id": 62085,
  "name": "Pilot",
  "episodeNumber": 1,
  "seasonNumber": 1,
  "airDate": "2008-01-20",
  "runtime": 58,
  "voteAverage": 7.8,
  "credits": { "cast": [], "crew": [] }
}
```

### Get Person

```http
GET http://localhost:8080/people/17419
```

```json
{
  "id": 17419,
  "imdbId": "nm0186505",
  "name": "Bryan Cranston",
  "biography": "Bryan Lee Cranston is an American actor...",
  "birthday": "1956-03-07",
  "currentAge": 70,
  "gender": "Male",
  "placeOfBirth": "Canoga Park, California, USA",
  "photoPath": "/7Jahy5LZX2Fo8fGJltMreAI49hC.jpg",
  "knownFor": "Acting",
  "movieCredits": [],
  "seriesCredits": []
}
```

### Get Person Series Credits

```http
GET http://localhost:8080/series/1396/person/17419/credits
```

```json
{
  "person": { "id": 17419, "name": "Bryan Cranston" },
  "series": { "id": 1396, "name": "Breaking Bad" },
  "totalEpisodeCount": 62,
  "roles": [{ "character": "Walter White" }],
  "seasons": [
    {
      "seasonNumber": 1,
      "name": "Season 1",
      "totalEpisodes": 7,
      "episodes": [
        { "episodeNumber": 1, "name": "Pilot", "airDate": "2008-01-20" }
      ]
    }
  ]
}
```

---

## 3. Auth Flow

Authentication uses OpenID Connect via `zitadel/oidc`. The library manages state, PKCE, and ID token verification.

### Begin OAuth (browser redirect)

Navigate in a browser — this redirects to the OAuth provider:

```http
GET http://localhost:8080/auth/google
```

```http
GET http://localhost:8080/auth/apple
```

The callback (`GET /auth/{provider}/callback` or `POST` for Apple's form_post) is handled automatically and redirects to the UI with a `?code=` parameter.

### Exchange Code for Session Token

```http
POST http://localhost:8080/auth/token
Authorization: Bearer <guest-key>
Content-Type: application/json
```

```json
{
  "code": "<code-from-callback-redirect>"
}
```

Response: `200 OK`

```json
{
  "session_token": "abc123..."
}
```

---

## 4. Session

All session routes require the session cookie.

**Required header:**

```
Cookie: mm_session_dev=<session-token>
```

### Get Session

```http
GET http://localhost:8080/auth/session
Cookie: mm_session_dev=<session-token>
```

```json
{
  "user": {
    "id": "usr_abc123",
    "username": null,
    "givenName": "John",
    "avatarUrl": "https://example.com/avatar.jpg"
  },
  "unseenAchievementCount": 0
}
```

### Logout

```http
POST http://localhost:8080/auth/logout
Cookie: mm_session_dev=<session-token>
```

Response: `204 No Content`

---

## 5. Watchlist

**Required header:**

```
Cookie: mm_session_dev=<session-token>
```

### Add to Watchlist

```http
POST http://localhost:8080/users/me/watchlist
Cookie: mm_session_dev=<session-token>
Content-Type: application/json
```

```json
{
  "mediaType": "movie",
  "mediaId": 550,
  "status": "want_to_watch"
}
```

Valid `mediaType`: `movie`, `series`
Valid `status`: `want_to_watch`, `watched`

Response: `201 Created`

```json
{
  "id": "wl_abc123",
  "mediaType": "movie",
  "mediaId": 550,
  "status": "want_to_watch",
  "title": "Fight Club",
  "posterPath": "/pB8BM7pdSp6B6Ih7QI4S2t0POoF.jpg"
}
```

### List Watchlist

```http
GET http://localhost:8080/users/me/watchlist
Cookie: mm_session_dev=<session-token>
```

Optional query params: `status=want_to_watch`, `media_type=movie`

```json
{
  "items": [],
  "total": 0
}
```

### Check if Item Exists

```http
GET http://localhost:8080/users/me/watchlist/check?media_type=movie&media_id=550
Cookie: mm_session_dev=<session-token>
```

```json
{
  "exists": true,
  "item": {
    "id": "wl_abc123",
    "status": "want_to_watch"
  }
}
```

### Update Watchlist Status

```http
PATCH http://localhost:8080/users/me/watchlist/wl_abc123
Cookie: mm_session_dev=<session-token>
Content-Type: application/json
```

```json
{
  "status": "watched"
}
```

Response: `200 OK` with the updated item.

### Remove from Watchlist

```http
DELETE http://localhost:8080/users/me/watchlist/wl_abc123
Cookie: mm_session_dev=<session-token>
```

Response: `204 No Content`

---

## 6. Watch Events

**Required header:**

```
Cookie: mm_session_dev=<session-token>
```

### Create Watch Event (Movie)

```http
POST http://localhost:8080/users/me/watch-events
Cookie: mm_session_dev=<session-token>
Content-Type: application/json
```

```json
{
  "mediaType": "movie",
  "mediaId": 550,
  "justWatched": true,
  "timezone": "America/Chicago"
}
```

Response: `201 Created`

```json
{
  "id": "we_abc123",
  "mediaType": "movie",
  "mediaId": 550,
  "watchedAt": "2026-04-28T20:30:00Z",
  "timezone": "America/Chicago"
}
```

### Create Watch Event (Episode)

```http
POST http://localhost:8080/users/me/watch-events
Cookie: mm_session_dev=<session-token>
Content-Type: application/json
```

```json
{
  "mediaType": "episode",
  "mediaId": 62085,
  "seriesId": 1396,
  "seasonNumber": 1,
  "episodeNumber": 1,
  "justWatched": true,
  "timezone": "America/Chicago"
}
```

Valid `mediaType`: `movie`, `episode`, `series`, `season`
For `episode` type, `seriesId`, `seasonNumber`, and `episodeNumber` are required.
For `season` type, `seriesId` and `seasonNumber` are required.

### Create Watch Event (Series — marks entire series watched)

```http
POST http://localhost:8080/users/me/watch-events
Cookie: mm_session_dev=<session-token>
Content-Type: application/json
```

```json
{
  "mediaType": "series",
  "mediaId": 1396,
  "justWatched": false,
  "timezone": "America/Chicago"
}
```

### List Watch Events

```http
GET http://localhost:8080/users/me/watch-events
Cookie: mm_session_dev=<session-token>
```

Optional query params: `media_type=movie`, `media_id=550`, `series_id=1396`, `dated_only=true`, `limit=50`

```json
{
  "events": [],
  "total": 0
}
```

### Delete Watch Event

```http
DELETE http://localhost:8080/users/me/watch-events/we_abc123
Cookie: mm_session_dev=<session-token>
```

Response: `204 No Content`

---

## 7. Episode Progress

**Required header:**

```
Cookie: mm_session_dev=<session-token>
```

### Mark Episode Watched

```http
POST http://localhost:8080/users/me/progress
Cookie: mm_session_dev=<session-token>
Content-Type: application/json
```

```json
{
  "seriesId": 1396,
  "seasonNumber": 1,
  "episodeNumber": 1,
  "timezone": "America/Chicago"
}
```

Response: `201 Created`

### Get Watch Progress for a Series

```http
GET http://localhost:8080/users/me/progress/1396
Cookie: mm_session_dev=<session-token>
```

```json
{
  "seriesId": 1396,
  "episodes": [
    {
      "id": "we_abc123",
      "seasonNumber": 1,
      "episodeNumber": 1,
      "watchedAt": "2026-04-28T20:30:00Z"
    }
  ],
  "total": 1
}
```

### Unmark Episode

```http
DELETE http://localhost:8080/users/me/progress/we_abc123
Cookie: mm_session_dev=<session-token>
```

Response: `204 No Content`

---

## 8. Stats & Achievements

**Required header:**

```
Cookie: mm_session_dev=<session-token>
```

### Get Stats

```http
GET http://localhost:8080/users/me/stats
Cookie: mm_session_dev=<session-token>
```

Response: `200 OK` with computed stats object.

### List Achievements

```http
GET http://localhost:8080/users/me/achievements
Cookie: mm_session_dev=<session-token>
```

```json
{
  "achievements": [
    {
      "id": "ach_abc123",
      "achievementId": "opening_credits",
      "name": "Opening Credits",
      "description": "Watch your first movie",
      "earnedViaMediaType": "movie",
      "earnedViaMediaId": 550,
      "earnedViaMediaTitle": "Fight Club",
      "seenAt": null,
      "earnedAt": "2026-04-28T20:30:00Z"
    }
  ]
}
```

### Get Unseen Achievements

```http
GET http://localhost:8080/users/me/achievements/unseen
Cookie: mm_session_dev=<session-token>
```

Same response shape as List Achievements, filtered to unseen only.

```json
{
  "achievements": []
}
```

### Mark Achievements Seen

```http
PATCH http://localhost:8080/users/me/achievements/seen
Cookie: mm_session_dev=<session-token>
```

Response: `204 No Content`

---

## 9. Account

**Required header:**

```
Cookie: mm_session_dev=<session-token>
```

### Export User Data

```http
POST http://localhost:8080/users/me/export
Cookie: mm_session_dev=<session-token>
```

Response: `200 OK` with `Content-Disposition: attachment; filename="minimovie-data-export.json"`

```json
{
  "account": {
    "username": null,
    "givenName": "John",
    "avatarUrl": "https://example.com/avatar.jpg",
    "providers": ["google"],
    "createdAt": "2026-01-15T10:00:00Z"
  },
  "watchlist": [],
  "watchEvents": [],
  "achievements": []
}
```

Rate limited to 1 export per day.

### Delete Account

```http
POST http://localhost:8080/users/me/delete
Cookie: mm_session_dev=<session-token>
Content-Type: application/json
```

```json
{
  "confirm": "delete"
}
```

Response: `200 OK`

```json
{
  "deleted": true
}
```

---

## 10. Apple Server-to-Server Notifications

This endpoint is called by Apple when a user revokes consent or deletes their Apple account.
It is not called by the client.

```http
POST http://localhost:8080/auth/apple/notifications
Content-Type: application/json
```

```json
{
  "payload": "<JWS-signed-notification-from-apple>"
}
```

Response: `200 OK` (notification processed)

The API verifies the JWS signature against Apple's JWKS, validates issuer/audience/iat claims,
deduplicates by JTI, and deletes the user account on `consent-revoked` or `account-delete` events.
