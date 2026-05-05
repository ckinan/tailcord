package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	serviceName = "tailcord"
	discordAPI  = "https://discord.com/api/v10"
)

type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Message struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Author    struct {
		Username string `json:"username"`
	} `json:"author"`
	Embeds []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Fields      []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"fields"`
		Footer struct {
			Text string `json:"text"`
		} `json:"footer"`
	} `json:"embeds"`
}

// gatewayPayload is every message sent/received over the WebSocket
// D holds event data as raw JSON, we defer parsing until we know the op code
// ref: https://docs.discord.com/developers/events/gateway
type gatewayPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
	T  string          `json:"t,omitempty"` // event name e.g. MESSAGE_CREATE
	S  int             `json:"s,omitempty"` // sequence number
}

// gatewayHello is the data inside op:10 HELLO
// ref: https://docs.discord.com/developers/events/gateway#hello-event
type gatewayHello struct {
	HeartbeatInterval int `json:"heartbeat_interval"` // milliseconds
}

type messageCreateEvent = Message

func main() {
	channelName := flag.String("channel", "", "Discord channel name to tail")
	n := flag.Int("n", 10, "Number of messages to show (default: 10)")
	follow := flag.Bool("f", false, "Follow channel, stream new messages live")

	flag.Parse()
	if len(flag.Args()) > 0 && flag.Args()[0] == "auth" {
		runAuth()
		return
	}
	if *channelName == "" {
		fmt.Fprintln(os.Stderr, "usage: tailcord --channel=NAME [-n N] [-f]")
		os.Exit(1)
	}

	token, err := keyring.Get(serviceName, "token")
	if err != nil {
		fmt.Fprintln(os.Stderr, "no token found, run `tailcord auth` first")
		os.Exit(1)
	}
	guildID, err := keyring.Get(serviceName, "guild_id")
	if err != nil {
		fmt.Fprintln(os.Stderr, "no guild ID found, run `tailcord auth` first")
		os.Exit(1)
	}

	channelID, err := resolveChannel(token, guildID, *channelName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *follow {
		if err := followChannel(token, channelID, *n); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	messages, err := fetchMessages(token, channelID, *n)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// print in chronological order (Discord returns newest first)
	for i := len(messages) - 1; i >= 0; i-- {
		printMessage(messages[i])
	}
}

func resolveChannel(token, guildID, channelName string) (string, error) {
	url := fmt.Sprintf("%s/guilds/%s/channels", discordAPI, guildID)

	body, err := discordGET(token, url)
	if err != nil {
		return "", err
	}

	var channels []Channel
	if err := json.Unmarshal(body, &channels); err != nil {
		return "", fmt.Errorf("parsing channels: %w", err)
	}

	for _, ch := range channels {
		if ch.Name == channelName {
			return ch.ID, nil
		}
	}
	return "", fmt.Errorf("channel %q not found in guild", channelName)
}

func fetchMessages(token, channelID string, limit int) ([]Message, error) {
	url := fmt.Sprintf("%s/channels/%s/messages?limit=%d", discordAPI, channelID, limit)

	body, err := discordGET(token, url)
	if err != nil {
		return nil, err
	}

	var messages []Message
	if err := json.Unmarshal(body, &messages); err != nil {
		return nil, fmt.Errorf("parsing messages: %w", err)
	}
	return messages, nil
}

func discordGET(token, url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bot "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var buf []byte
	tmp := make([]byte, 512)

	for {
		n, readErr := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if readErr != nil {
			break
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Discord API error %d: %s", resp.StatusCode, string(buf))
	}

	return buf, nil
}

// gatewayURL calls GET /gateway and returns the WebSocket URL
// Discord may change this URL, so we always fetch it fresh
func gatewayURL(token string) (string, error) {
	body, err := discordGET(token, discordAPI+"/gateway")
	if err != nil {
		return "", err
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing gateway URL: %w", err)
	}
	return result.URL, nil
}

func printMessage(m Message) {
	t, err := time.Parse(time.RFC3339Nano, m.Timestamp)
	ts := m.Timestamp
	if err == nil {
		ts = t.UTC().Format("2006-01-02 15:04:05")
	}

	if m.Content != "" {
		fmt.Printf("[%s] %s: %s\n", ts, m.Author.Username, m.Content)
	}

	for _, e := range m.Embeds {
		fmt.Printf("[%s] %s [embed]:\n", ts, m.Author.Username)
		if e.Title != "" {
			fmt.Printf("\tTitle: %s\n", e.Title)
		}
		if e.Description != "" {
			fmt.Printf("\tDescription: %s\n", e.Description)
		}
		for _, f := range e.Fields {
			fmt.Printf("\t- %s: %s\n", f.Name, f.Value)
		}
		if e.Footer.Text != "" {
			fmt.Printf("Footer: %s\n", e.Footer.Text)
		}
	}
}

// snowflakeAfter reports whether Discord message ID "a" is newer than message "b"
// IDs are large decimal integers, must parse as uint64, not compare as strings
func snowflakeAfter(a, b string) bool {
	ai, _ := strconv.ParseUint(a, 10, 64)
	bi, _ := strconv.ParseUint(b, 10, 64)
	return ai > bi
}

func runAuth() {
	token := promptSecret("Paste your Discord bot token: ")
	guildID := prompt("Paste your Discord guild (server) ID: ")

	if err := keyring.Set(serviceName, "token", token); err != nil {
		fmt.Fprintf(os.Stderr, "error storing token: %v\n", err)
		os.Exit(1)
	}

	if err := keyring.Set(serviceName, "guild_id", guildID); err != nil {
		fmt.Fprintf(os.Stderr, "error storing guild ID: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("credentials saved to keychain")
}

func prompt(message string) string {
	fmt.Print(message)
	reader := bufio.NewReader(os.Stdin)
	// ReadString reads until it hits '\n' (new line / enter key)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func promptSecret(message string) string {
	fmt.Print(message)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
		os.Exit(1)
	}
	return strings.TrimSpace(string(b))
}

func followChannel(token, channelID string, n int) error {
	wsURL, err := gatewayURL(token)
	if err != nil {
		return fmt.Errorf("getting gateway URL: %w", err)
	}
	wsURL += "?v10&encoding=json"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("connecting to gateway: %w", err)
	}
	defer conn.Close()

	// read op:10 HELLO, first message Discord always sends
	var hello gatewayPayload
	if err := conn.ReadJSON(&hello); err != nil {
		return fmt.Errorf("reading HELLO: %w", err)
	}

	if hello.Op != 10 {
		return fmt.Errorf("expected op 10 (HELLO), got op %d", hello.Op)
	}
	var helloData gatewayHello
	if err := json.Unmarshal(hello.D, &helloData); err != nil {
		return fmt.Errorf("parsing HELLO data: %w", err)
	}

	// context.WithCancel gives us a way to signal all goroutines to stop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Ctrl+C / SIGTERM : cancel the context, stops heartbeat, event loop
	// buffered channel (size 1) prevents the signal being dropped if w're
	// not waiting on it at the exact moment it fires
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func(cancel context.CancelFunc) {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nexiting...")
		cancel()
		conn.Close() // unblock ReadJSON immediately
	}(cancel)

	// heartbeat: send op:1 every heartbeat_interval ms.
	go func() {
		ticker := time.NewTicker(time.Duration(helloData.HeartbeatInterval) * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := conn.WriteJSON(gatewayPayload{Op: 1, D: json.RawMessage("null")}); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	// send op:2 IDENTIFY, prove who we are and which events we want
	// intents bitmask: GUILD_MESSAGES (1<<9) + MESSAGE_CONTENT (1<<15) = 33280
	type identifyProps struct {
		OS      string `json:"os"`
		Browser string `json:"browser"`
		Device  string `json:"device"`
	}
	type identifyData struct {
		Token      string        `json:"token"`
		Intents    int           `json:"intents"`
		Properties identifyProps `json:"properties"`
	}
	identifyJSON, _ := json.Marshal(identifyData{
		Token:      "Bot " + token,
		Intents:    33280,
		Properties: identifyProps{OS: "linux", Browser: "tailcord", Device: "tailcord"},
	})
	if err := conn.WriteJSON(gatewayPayload{Op: 2, D: identifyJSON}); err != nil {
		return fmt.Errorf("sending IDENTIFY: %w", err)
	}

	// move all WS reads into a goroutine so the main goroutine is free to do
	// the REST fetch without missing events
	// 256 slows is enough headroom for any burst that arrives during the
	// ~200ms REST call (hopefully)
	eventCh := make(chan gatewayPayload, 256)
	go func() {
		defer close(eventCh)
		for {
			var payload gatewayPayload
			if err := conn.ReadJSON(&payload); err != nil {
				return // connection closed; eventCh will be closed by defer
			}
			select {
			case eventCh <- payload:
			case <-ctx.Done():
				return
			}
		}
	}()

	// wait for READY before fetching history
	// any events that arrive here are buffered in eventCh
	for payload := range eventCh {
		if payload.Op == 0 && payload.T == "READY" {
			break
		}
	}

	// Fetch history now, WS is live, so nothing is missed
	var lastID string
	if n > 0 {
		messages, err := fetchMessages(token, channelID, n)
		if err != nil {
			return fmt.Errorf("fetching history: %w", err)
		}
		for i := len(messages) - 1; i >= 0; i-- {
			printMessage(messages[i])
		}
		if len(messages) > 0 {
			lastID = messages[0].ID // newest first, used to skip duplicates
		}
	}

	// Process events from the channel
	// events that arrived during the REST fetch are already buffered
	// we drain them here before blocking on new ones... snowflakeAfter skips
	// anything already covered by history
	for {
		select {
		case <-ctx.Done():
			return nil
		case payload, ok := <-eventCh:
			if !ok {
				if ctx.Err() != nil {
					return nil // ctrl+c closed the connection
				}
				return fmt.Errorf("connection closed unexpectedly")
			}
			switch {
			case payload.Op == 11:
				// hearbeat ACK, no action
			case payload.Op == 0 && payload.T == "MESSAGE_CREATE":
				var msg messageCreateEvent
				if err := json.Unmarshal(payload.D, &msg); err != nil {
					continue
				}
				if msg.ChannelID != channelID || msg.Author.Username == "" {
					continue
				}
				if lastID != "" && !snowflakeAfter(msg.ID, lastID) {
					continue // already in history, skip
				}
				printMessage(msg)
			}
		}
	}
}
