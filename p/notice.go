package p

import (
	"context"
	"fmt"
	"strings"
)

type Notice struct {
	Available bool   `json:"available"`
	Content   string `json:"content,omitempty"`
	Message   string `json:"message,omitempty"`
}

func (p *Portal) Notice(ctx context.Context) (*Notice, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	raw, err := p.call(ctx, "notice/get_notice_content", nil)
	if err != nil {
		return nil, err
	}
	message := scalar(raw["msg"])
	if scalar(raw["result"]) == "0" && message == "读取通知公告内容为空" {
		return &Notice{Message: message}, nil
	}
	if _, err := operationResult(raw, "notice query"); err != nil {
		return nil, err
	}
	content := scalar(raw["content"])
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("portal notice succeeded without content")
	}
	return &Notice{Available: true, Content: content, Message: message}, nil
}
