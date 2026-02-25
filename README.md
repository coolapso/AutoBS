# autobs: Automated Business Summary 😏

<p align="center">
  <img src="build/autobs_nobg.png" width="300" />
</p>

![Proudly Vibe Coded - Neon Flame](https://vibecoded.fyi/badges/flat/main/proudly-vibe-coded-neon-flame.svg)
[![Release](https://github.com/coolapso/autobs/actions/workflows/release.yaml/badge.svg?branch=main)](https://github.com/coolapso/autobs/actions/workflows/release.yaml)
![GitHub Tag](https://img.shields.io/github/v/tag/coolapso/autobs?logo=semver&label=semver&labelColor=gray&color=green)
[![Go Report Card](https://goreportcard.com/badge/github.com/coolapso/autobs)](https://goreportcard.com/report/github.com/coolapso/autobs)
![GitHub Sponsors](https://img.shields.io/github/sponsors/coolapso?style=flat&logo=githubsponsors)
![Coded with GitHub Copilot](https://vibecoded.fyi/badges/flat/agents/github-copilot.svg)
![Built with Claude](https://vibecoded.fyi/badges/flat/llms/claude.svg)

A Purely vibe coded go CLI tool that fetches your daily GitHub commits, groups them by Jira ticket, summarizes them with an LLM, and posts professional status updates as Jira comments — automatically.

## How It Works

1. **Collect** — Searches GitHub for all commits you authored today (using the GitHub Search API with `author:{user} author-date:>={date}`)
2. **Parse** — Extracts `Jira-Ticket: PROJ-123` footers from commit messages
3. **Enrich** — Fetches each Jira ticket's title and description for LLM context
4. **Summarize** — Sends each ticket's commits (plus ticket context) to an LLM for a management-friendly summary (concurrently)
5. **Post** — Adds the summary as a comment on the corresponding Jira ticket
6. **Report** — Prints which tickets were updated or failed

## Prerequisites

- Go 1.21+
- A GitHub personal access token (or use `gh auth token` if you have the GitHub CLI)
- A Jira account with [API token](https://id.atlassian.com/manage-profile/security/api-tokens)
- An OpenAI, Gemini, or AWS Bedrock account (OpenAI and Gemini haven't been tested yet!)
- Git commits must be authored by the same user as the GitHub token
- Git commits must have a Jira ticket in the footer with the format `Jira-Ticket: PROJ-123`

## Installation

Pre-built binaries for Linux, macOS, and Windows are available on the [releases page](https://github.com/coolapso/autobs/releases).

### Linux / macOS

#### Arch based distros (AUR)

```bash
yay -S autobs-bin
```

#### Install script

> [!WARNING]
> Please note that curl to bash is not the most secure way to install any project. Please make sure you understand and trust the [install script](https://github.com/coolapso/autobs/blob/main/build/install.sh) before running it.

**Latest version:**
```bash
curl -s https://autobs.coolapso.sh/install.sh | sudo bash
```

**Specific version:**
```bash
curl -s https://autobs.coolapso.sh/install.sh | VERSION="v1.0.0" sudo bash
```

#### Manually

Grab the archive for your platform from the [releases page](https://github.com/coolapso/autobs/releases), extract it, and place the binary somewhere in your `$PATH`.

```bash
VERSION=$(curl -s "https://api.github.com/repos/coolapso/autobs/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
curl -LO https://github.com/coolapso/autobs/releases/download/${VERSION}/autobs_linux_amd64.tar.gz
tar -xzf autobs_linux_amd64.tar.gz
sudo mv autobs /usr/local/bin/
```

#### Build from source

```bash
git clone https://github.com/coolapso/autobs
cd autobs
go build -o autobs .
sudo mv autobs /usr/local/bin/
```

### Windows

Grab the latest `.zip` for Windows from the [releases page](https://github.com/coolapso/autobs/releases), extract the archive, and run `autobs.exe` from a terminal. Optionally add it to a folder in your `%PATH%` for convenience.

### macOS

Grab the latest `.tar.gz` for darwin from the [releases page](https://github.com/coolapso/autobs/releases) and extract it.

If macOS warns that the binary is damaged or from an unidentified developer, remove it from quarantine:

```bash
xattr -d com.apple.quarantine /path/to/autobs
```

## Configuration

All settings can be provided via **environment variables** or a **config file**. Environment variables always take precedence.

### Option 1 — Config file (recommended)

Run the interactive setup once:

```bash
./autobs configure
```

This prompts for each setting and saves them to `~/.config/autobs/config.json`. Secrets are masked when shown. You won't need to set env vars again.

### Option 2 — Environment variables

| Variable       | Description                                                                 |
|----------------|-----------------------------------------------------------------------------|
| `GITHUB_TOKEN` | GitHub personal access token. Use `$(gh auth token)` for private org repos  |
| `GITHUB_USER`  | Your GitHub username (must match your git commit author login)              |
| `JIRA_URL`     | Base URL of your Jira instance (e.g. `https://yourorg.atlassian.net`)      |
| `JIRA_USER`    | Your Jira account email                                                     |
| `JIRA_TOKEN`   | Jira API token — generate at https://id.atlassian.com/manage-profile/security/api-tokens |
| `LLM_PROVIDER` | LLM to use: `openai`, `gemini`, or `bedrock`                               |
| `LLM_API_KEY`  | API key for OpenAI or Gemini (not needed for Bedrock)                       |
| `LLM_MODEL`    | Model override (optional for OpenAI/Gemini, **required** for Bedrock)       |
| `AWS_REGION`   | AWS region for Bedrock (e.g. `us-east-1`) — required when using Bedrock     |

> **Tip:** For private organisation repos, use `GITHUB_TOKEN=$(gh auth token)` — the gh CLI token has the correct scopes.

## Usage

### First-time setup

```bash
./autobs configure
```

Prompts interactively for all settings and saves to `~/.config/autobs/config.json`. When Bedrock is selected it asks for region and model; for OpenAI/Gemini it asks for the API key and an optional model override.

### Run

```bash
./autobs
```

### Dry run (preview without posting to Jira)

```bash
./autobs --dry-run
```

Fetches commits and generates LLM summaries, but prints them to the terminal instead of posting to Jira.

### Using env vars instead

**OpenAI / Gemini:**

```bash
export GITHUB_TOKEN=$(gh auth token) GITHUB_USER=johndoe
export JIRA_URL=https://myorg.atlassian.net JIRA_USER=john@myorg.com JIRA_TOKEN=ATATT3x...
export LLM_PROVIDER=openai LLM_API_KEY=sk-...
./autobs
```

**AWS Bedrock:**

```bash
export GITHUB_TOKEN=$(gh auth token) GITHUB_USER=johndoe
export JIRA_URL=https://myorg.atlassian.net JIRA_USER=john@myorg.com JIRA_TOKEN=ATATT3x...
export LLM_PROVIDER=bedrock AWS_REGION=us-east-1
export LLM_MODEL=anthropic.claude-3-5-sonnet-20241022-v2:0
# AWS credentials from env or ~/.aws/credentials
./autobs
```

### Built-in help

```bash
./autobs --help
./autobs configure --help
```

## Commit Format

To link a commit to a Jira ticket, include a `Jira-Ticket` footer in the commit message:

```
feat: implement OAuth2 login flow

Adds support for Google and GitHub OAuth providers.
Replaces the legacy username/password-only login.

Jira-Ticket: AUTH-42
```

Commits without a `Jira-Ticket` footer are fetched but ignored during grouping.

## Example Output

**Normal run:**
```
Found 3 commit(s) from GitHub for user "johndoe" on 2026-02-24.
2 unique ticket(s) found: AUTH-42 PLAT-17

=== autobs Report ===
  [UPDATED] AUTH-42
  [UPDATED] PLAT-17
```

**Dry run:**
```
--- DRY RUN — nothing will be posted to Jira ---
Found 3 commit(s) from GitHub for user "johndoe" on 2026-02-24.
2 unique ticket(s) found: AUTH-42 PLAT-17

=== autobs Dry Run Preview ===

┌─ AUTH-42
│  • Completed OAuth2 login integration with Google and GitHub providers
│  • Replaced legacy password-only flow, improving security posture
└─ (not posted)
```

## Supported LLM Providers

| Provider  | `LLM_PROVIDER` | Auth              | Default model      |
|-----------|----------------|-------------------|--------------------|
| OpenAI    | `openai`       | `LLM_API_KEY`     | `gpt-4o-mini`      |
| Gemini    | `gemini`       | `LLM_API_KEY`     | `gemini-1.5-flash` |
| AWS Bedrock | `bedrock`    | AWS credential chain | *(must set `LLM_MODEL`)* |

Bedrock supports any model via the Converse API (Claude, Llama, Titan, etc.).

## Contributing

This is an AI Friendly project, and contributions are very welcome! Feel free to bring your favorite AI Pets along, if you want to look at the code yourself, feel free but I will not look at the code of this project EVER, it will be fully reviewed and managed by AI. The goal is to see how far can I take this. I will promise that I will disclose the day I actually have to look at the code and do changes myself!

## Architecture

The tool uses a provider pattern for extensibility:

```
internal/vcs/         — VCSProvider interface + GitHub implementation (go-github SDK)
internal/tracker/     — TrackerProvider interface + Jira implementation (PostComment + GetTicket)
internal/summarizer/  — Summarizer interface + OpenAI/Gemini/Bedrock implementation
pkg/models/           — Shared Commit, Summary, and TicketInfo types
cmd/                  — CLI entry point (cobra), Jira ticket extraction, orchestration
```

New VCS providers (e.g. GitLab) or trackers (e.g. Linear) can be added by implementing the respective interface. Jira ticket extraction (`Jira-Ticket: PROJ-123`) is decoupled from the VCS layer and lives in the orchestration layer.

The tool was coded with github copilot cli, and more information and details about the project can be found in the [copilot-instructions](.copilot/instructions.md) file.

---

If you like this project and want to support / contribute in a different way you can always [:heart: Sponsor Me](https://github.com/sponsors/coolapso) or

<a href="https://www.buymeacoffee.com/coolapso" target="_blank">
  <img src="https://cdn.buymeacoffee.com/buttons/default-yellow.png" alt="Buy Me A Coffee" style="height: 51px !important;width: 217px !important;" />
</a>
