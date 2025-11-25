---

## 🎯 Project Overview

**GoStream** - A video streaming service backend designed as a portfolio project demonstrating system design skills.

### Project Goals
- Understand of **trade-offs** in distributed systems
- Build for **scalability** under high traffic
- Design for **fault tolerance** and graceful degradation
- Implement comprehensive **observability**

---

## 🏗 System Architecture & Design Rules

### Tech Stack
* **Language:** Go (Latest Stable) - Chosen for high performance and native concurrency
* **Database:** PostgreSQL (Metadata)
* **Message Queue:** RabbitMQ (Task queue for async processing)
* **Object Storage:** MinIO (S3-compatible, local dev) / AWS S3 (Production)
* **Video Processing:** FFmpeg (HLS transcoding)
* **Infra:** Docker & Docker Compose
* **Testing:** Standard `testing` package with `go-sqlmock` / `gomock`

### Architecture Components
```
┌─────────────┐     ┌────────────┐     ┌─────────────┐
│   Client    │────▶│ API Server │────▶│  PostgreSQL │
└─────────────┘     └─────┬──────┘     └─────────────┘
                          │
                          │ Presigned URL
                          ▼
                   ┌─────────────┐
                   │    MinIO    │◀──── Direct Upload
                   └─────────────┘
                          │
      ┌───────────────────┼───────────────────┐
      │                   │                   │
      ▼                   ▼                   ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  RabbitMQ   │────▶│   Worker    │────▶│   FFmpeg    │
└─────────────┘     └─────────────┘     └─────────────┘
```

### Key Design Decisions

1. **Presigned URL Upload**
   - Client uploads directly to object storage
   - *Trade-off:* Reduces API server bandwidth/memory load at cost of slightly more complex client logic

2. **Async Transcoding via Message Queue**
   - API and Worker are decoupled
   - *Trade-off:* Eventually consistent, but allows independent scaling of CPU-intensive work

3. **HLS (HTTP Live Streaming)**
   - Segment-based streaming with .m3u8 manifests
   - *Trade-off:* More storage (multiple segments) but enables adaptive bitrate in future phases

---

## 📊 Database Schema

```sql
CREATE TABLE videos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    title VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL, -- PENDING_UPLOAD, PROCESSING, READY, FAILED
    original_url TEXT,
    hls_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_videos_user_id ON videos(user_id);
CREATE INDEX idx_videos_status ON videos(status);
```

### Video Status State Machine
```
PENDING_UPLOAD ──▶ PROCESSING ──▶ READY
                       │
                       └──▶ FAILED
```

---

## 🔌 API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/videos` | Create metadata & get presigned upload URL |
| `POST` | `/v1/videos/{id}/process` | Trigger transcoding (idempotent) |
| `GET` | `/v1/videos/{id}` | Get video info (includes HLS URL when READY) |
| `GET` | `/health` | Health check for k8s probes |

---

## 📝 Git & GitHub Guidelines

### Workflow: Tracking Issue for Major Milestones

**For Individual Development:** Use **Tracking Issues** to manage major features or releases without creating excessive granular issues.

**When to Create a Tracking Issue:**
- ✅ Major feature sets spanning multiple PRs (e.g., "Initial Release v1.0")
- ✅ Large refactoring efforts across multiple layers
- ✅ Coordinated changes requiring multiple PRs
- ❌ Small bug fixes or single-PR features (just create PR directly)

**How to Use:**
1.  **Create Tracking Issue** using `.github/ISSUE_TEMPLATE/tracking_issue.md`
2.  **List PRs** with task list format: `- [ ] #PR_NUMBER description`
3.  **Reference in PRs** with `Part of #<issue-number>` (NOT `Closes #<issue>`)
4.  **Track Progress** - GitHub shows `X of Y tasks completed`
5.  **Close Manually** when all PRs are merged

### Branching Strategy
* `main`: Always deployable. Protected branch (base for all features).
* **Rule:** NEVER commit directly to `main`. ALL changes must go through feature branches and Pull Requests.
* **Branch Naming Convention:**
    - `feat/*`: New features (e.g., `feat/user-auth`)
    - `fix/*`: Bug fixes (e.g., `fix/cache-race-condition`)
    - `test/*`: Test additions/improvements
    - `ci/*`: CI/CD pipeline changes
    - `docs/*`: Documentation updates
    - `refactor/*`: Code refactoring without behavior change
* **Workflow:**
    1. Create feature branch from `main`
    2. Commit changes with proper commit messages
    3. Push branch and create Pull Request
    4. After review/approval, merge to `main` (squash or merge commit)
    5. Delete feature branch after merge

### Commit Messages (The 50/72 Rule)
**MANDATORY Format:**
```text
<type>: <subject (50 chars max, imperative mood)>

<body (wrap at 72 chars, explain WHY not HOW)>
```

* **Imperative Mood:** "Add feature" (not "Added").
* **Granularity:** Atomic commits. One logical change per commit.

### Commit Granularity Guidelines
**Split changes into appropriate commits by logical unit:**
- Each commit should represent ONE logical change
- Separate infrastructure changes from application code
- Separate configuration from implementation
- Example split for API server:
  1. `feat: Add config management` - config package only
  2. `feat: Add HTTP middleware` - middleware implementations
  3. `feat: Add health check endpoint` - handler + route
  4. `chore: Add Dockerfile and docker-compose` - infrastructure

### Pull Requests (PRs)

**Template Usage:**
* Use `.github/pull_request_template.md` for all PRs
* Must be in **English**
* **Required Sections:**
    - **Summary:** Brief overview of changes
    - **Motivation & Context (Why?):** Explain the problem being solved and why this change is necessary
    - **Implementation Details (How?):** High-level technical approach and key architectural decisions

**Quality Standards:**
* Focus on **WHY** (motivation) over **WHAT** (code changes)
* Highlight trade-offs and technical decisions made
* Self-review before requesting review from others

-----

## 🛡 Coding Standards & Best Practices

### Go Specific Rules

1.  **Error Handling:**
    * Never ignore errors (`_`).
    * Use error wrapping: `fmt.Errorf("context: %w", err)` to preserve stack traces/context.
2.  **Testing:**
    * Table-driven tests are preferred.
    * Use interfaces for all external dependencies (DB, Cache) to facilitate mocking.
3.  **Concurrency:**
    * Prefer Channels and WaitGroups over Mutexes where applicable, but choose the simplest solution.
4.  **Linting:**
    * Code must be compliant with `golangci-lint` standard rules.

### Security

* **Secrets:** Never hardcode credentials. Use environment variables.
* **Validation:** Sanitize all inputs.

-----

## 🤖 Assistant Instructions

1.  **Persona:** Act as a Senior Engineer at a top-tier tech company. Be critical of implementation details regarding performance and scalability.
2.  **Explain Trade-offs:** When suggesting a solution, briefly explain *why* it is better than the alternative (e.g., "I chose `map` here for O(1) access, trading memory for speed").
3.  **Convention Enforcement:** If the user provides a commit message that violates the 50/72 rule or uses Japanese, **rewrite it** to follow the standard before proceeding.
4.  **Test First:** Remind the user about tests if they generate implementation code without corresponding tests.
5.  **No "Line Number" References:** When discussing code, refer to function names or logic blocks, as line numbers change.
6.  **No Claude Code Signature:** Do NOT add Claude Code signature or Co-Authored-By footer to commit messages.

-----

## 📁 Project Structure Pattern

* `.github/`: PR templates and GitHub Actions workflows.
* `cmd/`: Main applications.
* `internal/`: Private application and library code (Service, Repository).
* `api/`: OpenAPI/Swagger definitions.

