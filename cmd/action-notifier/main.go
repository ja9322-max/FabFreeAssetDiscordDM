package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fabfreeassetdiscorddm/internal/fab"
)

const discordAPI = "https://discord.com/api/v10"

type asset struct {
	Name string `json:"name"`
	Link string `json:"link"`
}

type state struct {
	Seen []string `json:"seen"`
}

func main() {
	if err := run(context.Background(), os.Getenv, http.DefaultClient); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, client *http.Client) error {
	token := strings.TrimSpace(getenv("DISCORD_BOT_TOKEN"))
	userID := strings.TrimSpace(getenv("DISCORD_USER_ID"))
	if token == "" || userID == "" {
		return errors.New("DISCORD_BOT_TOKEN and DISCORD_USER_ID are required")
	}

	statePath := strings.TrimSpace(getenv("STATE_PATH"))
	if statePath == "" {
		statePath = "data/seen-assets.json"
	}

	scraped, _, err := fab.GetFreeAssets(ctx, 3)
	if err != nil {
		return fmt.Errorf("fetch Fab assets: %w", err)
	}
	current := make([]asset, 0, len(scraped))
	for _, item := range scraped {
		if strings.TrimSpace(item.Link) != "" {
			current = append(current, asset{Name: item.Name, Link: item.Link})
		}
	}

	previous, err := loadState(statePath)
	if err != nil {
		return err
	}
	newAssets := unseen(current, previous.Seen)
	if len(newAssets) > 0 {
		if err := sendDM(ctx, client, token, userID, formatMessage(newAssets)); err != nil {
			return err
		}
		fmt.Printf("Sent %d new asset(s).\n", len(newAssets))
	} else {
		fmt.Println("No new assets.")
	}

	return saveState(statePath, append(previous.Seen, assetLinks(current)...))
}

func loadState(path string) (state, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state{}, nil
	}
	if err != nil {
		return state{}, fmt.Errorf("read state: %w", err)
	}
	var result state
	if err := json.Unmarshal(data, &result); err != nil {
		return state{}, fmt.Errorf("parse state: %w", err)
	}
	return result, nil
}

func unseen(current []asset, seen []string) []asset {
	known := make(map[string]struct{}, len(seen))
	for _, link := range seen {
		known[link] = struct{}{}
	}
	result := make([]asset, 0)
	for _, item := range current {
		if _, ok := known[item.Link]; !ok {
			result = append(result, item)
		}
	}
	return result
}

func assetLinks(assets []asset) []string {
	links := make([]string, 0, len(assets))
	for _, item := range assets {
		links = append(links, item.Link)
	}
	return links
}

func saveState(path string, links []string) error {
	links = unique(links)
	sort.Strings(links)
	data, err := json.MarshalIndent(state{Seen: links}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

func unique(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func formatMessage(assets []asset) string {
	var message strings.Builder
	message.WriteString("🎁 **Fab 기간 한정 무료 에셋이 등록됐어요!**\n")
	for _, item := range assets {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = "Fab 무료 에셋"
		}
		fmt.Fprintf(&message, "\n• **%s**\n%s\n", name, item.Link)
	}
	return message.String()
}

func sendDM(ctx context.Context, client *http.Client, token, userID, content string) error {
	channelID, err := createDM(ctx, client, token, userID)
	if err != nil {
		return err
	}
	return discordRequest(ctx, client, token, http.MethodPost, discordAPI+"/channels/"+channelID+"/messages", map[string]string{"content": content}, nil)
}

func createDM(ctx context.Context, client *http.Client, token, userID string) (string, error) {
	var response struct {
		ID string `json:"id"`
	}
	if err := discordRequest(ctx, client, token, http.MethodPost, discordAPI+"/users/@me/channels", map[string]string{"recipient_id": userID}, &response); err != nil {
		return "", err
	}
	if response.ID == "" {
		return "", errors.New("Discord returned an empty DM channel ID")
	}
	return response.ID, nil
}

func discordRequest(ctx context.Context, client *http.Client, token, method, url string, body any, result any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Discord request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Discord returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode Discord response: %w", err)
		}
	}
	return nil
}
