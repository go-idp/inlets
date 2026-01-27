package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-idp/inlets/internal/client"
)

// Notifier handles sending notifications to various services
type Notifier struct {
	config *client.NotificationConfig
	client *http.Client
}

// NewNotifier creates a new notifier instance
func NewNotifier(config *client.NotificationConfig) *Notifier {
	if config == nil {
		return nil
	}

	return &Notifier{
		config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Notify sends a notification with title and message
func (n *Notifier) Notify(title string, message []string) error {
	if n == nil || n.config == nil {
		return nil // No notification configured
	}

	// Join message lines
	content := strings.Join(message, "\n")

	// Send message based on provider
	switch strings.ToLower(n.config.Provider) {
	case "dingtalk", "ding":
		return n.sendToDingTalk(title, content)
	case "feishu", "lark":
		return n.sendToFeishu(title, content)
	case "wecom", "wechat", "wework":
		return n.sendToWeCom(title, content)
	case "slack":
		return n.sendToSlack(title, content)
	default:
		return fmt.Errorf("unsupported notification provider: %s", n.config.Provider)
	}
}

// sendToDingTalk sends a notification to DingTalk webhook
func (n *Notifier) sendToDingTalk(title string, content string) error {
	// DingTalk webhook format
	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  fmt.Sprintf("## %s\n\n%s", title, content),
		},
	}

	return n.sendHTTPRequest(n.config.URL, payload)
}

// sendToFeishu sends a notification to Feishu webhook
func (n *Notifier) sendToFeishu(title string, content string) error {
	// Feishu webhook format
	payload := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"config": map[string]interface{}{
				"wide_screen_mode": true,
			},
			"header": map[string]interface{}{
				"title": map[string]string{
					"tag":     "plain_text",
					"content": title,
				},
				"template": "blue",
			},
			"elements": []map[string]interface{}{
				{
					"tag": "div",
					"text": map[string]interface{}{
						"tag":     "lark_md",
						"content": content,
					},
				},
			},
		},
	}

	return n.sendHTTPRequest(n.config.URL, payload)
}

// sendToWeCom sends a notification to WeCom (企业微信) webhook
func (n *Notifier) sendToWeCom(title string, content string) error {
	// WeCom webhook format
	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": fmt.Sprintf("# %s\n\n%s", title, content),
		},
	}

	return n.sendHTTPRequest(n.config.URL, payload)
}

// sendToSlack sends a notification to Slack webhook
func (n *Notifier) sendToSlack(title string, content string) error {
	// Slack webhook format
	payload := map[string]interface{}{
		"text": title,
		"blocks": []map[string]interface{}{
			{
				"type": "header",
				"text": map[string]interface{}{
					"type": "plain_text",
					"text": title,
				},
			},
			{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": content,
				},
			},
		},
	}

	return n.sendHTTPRequest(n.config.URL, payload)
}

// sendHTTPRequest sends an HTTP POST request with JSON payload
func (n *Notifier) sendHTTPRequest(url string, payload interface{}) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
