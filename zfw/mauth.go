package zfw

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

type MauthPolicy struct {
	State string `json:"state"`
}

type MauthChange struct {
	State         string  `json:"state"`
	PreviousState string  `json:"previous_state"`
	Outcome       Outcome `json:"outcome"`
	Verified      bool    `json:"verified"`
}

func (s *Session) Mauth(ctx context.Context) (*MauthPolicy, error) {
	var content string
	if err := s.json(ctx, "dashboard/refreshMauthType", nil, &content); err != nil {
		return nil, err
	}
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, err
	}
	link := find(doc, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "a" })
	if link == nil || attr(link, "href") != "dashboard/oprateMauthAction" || nodeText(link) == "" {
		return nil, fmt.Errorf("Self mauth response does not contain its action link and state")
	}
	return &MauthPolicy{State: nodeText(link)}, nil
}

func (s *Session) ChangeMauth(ctx context.Context) (*MauthChange, error) {
	before, err := s.Mauth(ctx)
	if err != nil {
		return &MauthChange{Outcome: NotSubmitted}, err
	}
	result := &MauthChange{PreviousState: before.State, Outcome: Unknown}
	data, final, _, err := s.request(ctx, http.MethodGet, "dashboard/oprateMauthAction", nil)
	if err != nil {
		return result, fmt.Errorf("mauth operation failed; result is unknown: %w", err)
	}
	if _, err := s.authenticatedPage(data, final); err != nil {
		return result, err
	}
	after, err := s.Mauth(ctx)
	if err != nil {
		return result, err
	}
	result.State = after.State
	if strings.TrimSpace(before.State) == strings.TrimSpace(after.State) {
		return result, fmt.Errorf("Self mauth action did not change the displayed state")
	}
	result.Outcome, result.Verified = Accepted, true
	return result, nil
}
