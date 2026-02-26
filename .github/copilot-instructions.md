# updateMyTickets: Copilot CLI Instructions (Extensible Architecture)

## Project Overview
`updateMyTickets` is a Go-based CLI tool that aggregates a developer's daily GitHub commit activity, summarizes it via an LLM, and posts the professional summary as a comment on the relevant Jira tickets. The architecture is decoupled via interfaces to allow future support for different providers (e.g., GitLab, Linear).

---

## Core Architecture

### Provider Pattern Interfaces
* **VCSProvider** (`internal/vcs/provider.go`): `GetCommits(since time.Time, user string) ([]models.Commit, error)`
* **Summarizer** (`internal/summarizer/summarizer.go`): `Summarize(commits []string, ticketTitle, ticketDescription string) (string, error)`
* **TrackerProvider** (`internal/tracker/provider.go`): `PostComment(ticketID string, body string) error`, `GetTicket(ticketID string) (*models.TicketInfo, error)`

### Shared Types (`pkg/models/models.go`)
* `Commit { SHA, Message string }` — raw commit from VCS, no ticket info
* `Summary { TicketID, Body string }`
* `TicketInfo { Title, Description string }` — ticket metadata fetched from the tracker for LLM context

---

## Implemented Providers

### GitHub VCS (`internal/vcs/github.go`)
* **SDK:** `github.com/google/go-github/v65/github` + `golang.org/x/oauth2`
* **API:** GitHub Search API (`client.Search.Commits`)
* **Query (today):** `author:{GITHUB_USER} author-date:>={today}` — open-ended, matches everything from today onward
* **Query (bounded/yesterday):** `author:{GITHUB_USER} author-date:{since}..{until}` — uses GitHub's inclusive range syntax; two separate `author-date` qualifiers are not reliably AND-ed by the API
* **Note:** `author:` is used (not `committer:`) because commits are often merged via PR/CI

### LLM Summarizer (`internal/summarizer/llm.go`)
* Supports `openai`, `gemini`, and `bedrock` via `LLM_PROVIDER`
* **`systemPrompt`:** professional, management-friendly status update; plain text only (no markdown, no bullet symbols), short sentences separated by newlines
* **`standupSystemPrompt`:** informal, technical standup style; conversational tone, plain text, short paragraphs separated by newlines
* The three provider methods (`summarizeOpenAI`, `summarizeGemini`, `summarizeBedrock`) each accept a `sysPrompt` parameter via the shared `summarize()` helper
* **`Summarize(commits, ticketTitle, ticketDescription)`** — uses `systemPrompt`; prepends Jira ticket context when available
* **`SummarizeStandup(commits)`** — uses `standupSystemPrompt`; accepts all commits regardless of Jira-Ticket footer
* OpenAI default model: `gpt-4o-mini`; Gemini default: `gemini-1.5-flash`
* Bedrock uses the **Converse API** (`bedrockruntime.Converse`) — works across all model families; `LLM_MODEL` is required
* AWS credentials for Bedrock use the standard chain (env vars or `~/.aws/credentials`)
* **Concurrency:** each ticket processed in its own goroutine via `sync.WaitGroup`

### Jira Tracker (`internal/tracker/jira.go`)
* **`PostComment`:** `POST /rest/api/3/issue/{ticketID}/comment` — body in Jira ADF
* **`GetTicket`:** `GET /rest/api/3/issue/{ticketID}?fields=summary,description` — fetches ticket title and ADF description; ADF is flattened to plain text via `extractADFText`
* **Auth:** HTTP Basic Auth (email + API token)
* **Failure handling:** if `GetTicket` fails, a warning is logged and summarization proceeds without ticket context

---

## CLI (`cmd/`)

### Commands
* `autobs` (default) — run the full pipeline
* `autobs configure` — interactive setup that saves to `~/.autobs.json`

### Flags
* `--dry-run` — generates LLM summaries but prints them to terminal instead of posting to Jira
* `--yesterday` — targets yesterday's date instead of today; uses `author-date:SINCE..UNTIL` range query
* `--standup` — prints an informal standup-style summary of **all** commits (no Jira-Ticket filter, no Jira posting required)

### Configuration Resolution
Settings are resolved in this order (first wins):
1. Environment variable
2. Config file (`~/.updateMyTickets.json`)

The config file is written with `0600` permissions. Secrets are masked in the configure prompt.

### Environment Variables
| Variable       | Required for        | Notes |
|----------------|---------------------|-------|
| `GITHUB_TOKEN` | always              | Use `$(gh auth token)` for private org repos |
| `GITHUB_USER`  | always              | Must match git commit author login |
| `JIRA_URL`     | non-standup         |  |
| `JIRA_USER`    | non-standup         | Jira account email |
| `JIRA_TOKEN`   | non-standup         |  |
| `LLM_PROVIDER` | always              | `openai`, `gemini`, or `bedrock` |
| `LLM_API_KEY`  | openai / gemini     | Not needed for Bedrock |
| `LLM_MODEL`    | bedrock (required), openai/gemini (optional) | |
| `AWS_REGION`   | bedrock             |  |

---

## Implementation Logic Flow

1. **Initialize** — load config (env → file fallback), validate required fields per provider (Jira fields skipped for `--standup`), instantiate providers
2. **Collect** — `VCS.GetCommits(since, until, user)` returns raw commits (SHA + Message); `since`/`until` are local-midnight times
3. **Standup branch** — if `--standup`: pass all commit messages to `Summarizer.SummarizeStandup()`, print result, exit (no Jira interaction)
4. **Extract** — regex `Jira-Ticket:\s*([A-Z]+-\d+)` applied in `cmd/root.go` (decoupled from VCS layer); group by ticket ID
5. **Process** — for each ticket concurrently: `Tracker.GetTicket(ticketID)` (for context) → `Summarizer.Summarize(messages, title, description)` → `Tracker.PostComment(ticketID, summary)`
6. **Report** — print `[UPDATED]` / `[FAILED]` per ticket; dry-run prints formatted preview instead

---

## Development Standards

* **Formatting:** All Go code must be formatted with `go fmt` before committing.
* **Linting:** All `golangci-lint` checks must be addressed before committing. Run `golangci-lint run` and resolve any reported issues.
* **Commits:** A commit must be made after each relevant step (e.g., after adding a feature, fixing a bug, updating config, adding tests). Commits must follow the [Conventional Commits](https://www.conventionalcommits.org/) standard:
  * `feat:` — new feature
  * `fix:` — bug fix
  * `build:` — maintenance, tooling, dependencies
  * `chore:` — web page related changes
  * `docs:` — documentation changes
  * `refactor:` — code restructuring without behaviour change
  * `test:` — adding or updating tests
  * `ci:` — CI/CD changes
* **Language:** Go 1.21+
* **CLI:** `github.com/spf13/cobra`
* **GitHub SDK:** `github.com/google/go-github/v65/github` + `golang.org/x/oauth2`
* **AWS SDK:** `github.com/aws/aws-sdk-go-v2` + `bedrockruntime`
* **Error handling:** per-ticket errors are logged but do not stop processing of other tickets
