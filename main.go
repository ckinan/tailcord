package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	serviceName = "cktail"
	discordAPI  = "https://discord.com/api/v10"
)

type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Message struct {
	ID        string `json:"id"`
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

func main() {
	channelName := flag.String("channel", "", "Discord channel name to tail")
	n := flag.Int("n", 10, "Number of messages to show (default: 10)")

	flag.Parse()
	if len(flag.Args()) > 0 && flag.Args()[0] == "auth" {
		runAuth()
		return
	}
	if *channelName == "" {
		fmt.Fprintln(os.Stderr, "usage: cktail --channel=NAME")
		os.Exit(1)
	}

	token, err := keyring.Get(serviceName, "token")
	if err != nil {
		fmt.Fprintln(os.Stderr, "no token found, run `cktail auth` first")
		os.Exit(1)
	}
	guildID, err := keyring.Get(serviceName, "guild_id")
	if err != nil {
		fmt.Fprintln(os.Stderr, "no guild ID found, run `cktail auth` first")
		os.Exit(1)
	}

	channelID, err := resolveChannel(token, guildID, *channelName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
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
