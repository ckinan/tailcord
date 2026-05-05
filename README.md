# tailcord

A `tail`-inspired CLI for streaming Discord channel messages in the terminal, written in Go.

```
tailcord --channel=lab-alerts -n 10 -f
```

![demo](demo.gif)

---

## Motivation

I use Discord as a notification server for my side-project microservices. I'd rather
watch alerts in a persistent terminal window than keep the Discord app open. `tailcord`
shows plain-text messages from a single channel - nothing more.

---

## Installation

```bash
go install github.com/ckinan/tailcord@latest
```

Make sure `$GOPATH/bin` is on your `$PATH` (usually `~/go/bin`):

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Upgrade

```bash
GOPROXY=direct go install github.com/ckinan/tailcord@latest
```

---

## Discord setup

`tailcord` connects using a Discord Bot token. Follow these steps once:

### 1. Create a Bot

1. Go to [discord.com/developers/applications](https://discord.com/developers/applications)
2. Click **New Application** -> give it a name (e.g. `tailcord`)
3. Go to **Bot** in the left sidebar
4. Click **Reset Token** and copy the token - you will need it in the next section

### 2. Enable the Message Content intent

Still on the **Bot** page, scroll down to **Privileged Gateway Intents** and enable:

- Message Content Intent

Without this, the bot can receive events but message text will always be empty.

### 3. Invite the bot to your server

1. Go to **OAuth2 -> URL Generator**
2. Scopes: check `bot`
3. Bot Permissions: check `Read Message History/View Channels`
4. Copy the generated URL, open it in a browser, and add the bot to your server

### 4. Get your Guild (server) ID

Right-click your server name in Discord -> **Copy Server ID**.
(If you don't see this option, enable Developer Mode in Settings -> Advanced.)

### 5. Authenticate

```bash
tailcord auth
# Paste your Discord bot token: (hidden)
# Paste your Discord guild (server) ID: 123456789012345678
# credentials saved to keychain
```

Credentials are stored in your OS keychain (macOS Keychain, GNOME Keyring, or
Windows Credential Manager) - you only do this once per machine.

---

## Usage

```bash
# One-time auth
tailcord auth

# Print last 10 messages and exit (default)
tailcord --channel=lab-alerts

# Print last N messages and exit
tailcord --channel=lab-alerts -n 20

# Stream new messages live (no history)
tailcord --channel=lab-alerts -f

# Print last N messages, then stream live (most useful)
tailcord --channel=lab-alerts -n 5 -f

# Pipe / filter
tailcord --channel=lab-alerts -f | grep "ERROR"
tailcord --channel=lab-alerts -n 50 | grep "alertbot"
```

---

## Output format

Plain text, one message per line, fully `grep`/pipe-able:

```
[2026-05-05 14:00:01] alertbot: CPU alert: 95% on prod-01
[2026-05-05 14:00:42] alertbot: Memory alert: 88% on prod-02
[2026-05-05 14:01:10] you: acknowledged
```
