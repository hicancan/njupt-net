package zfw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type ConsumeLimit struct {
	Limit     json.Number `json:"limit"`
	Unlimited bool        `json:"unlimited"`
}

type ConsumeChange struct {
	*ConsumeLimit
	RequestedLimit json.Number `json:"requested_limit"`
	Outcome        Outcome     `json:"outcome"`
	Verified       bool        `json:"verified"`
}

func (s *Session) ConsumeProtect(ctx context.Context) (*ConsumeLimit, error) {
	doc, err := s.page(ctx, "service/consumeProtect")
	if err != nil {
		return nil, err
	}
	return consumeProtect(doc)
}

func consumeProtect(doc *html.Node) (*ConsumeLimit, error) {
	if findForm(doc, "/Self/service/changeConsumeProtect") == nil {
		return nil, fmt.Errorf("Self consume protection form is missing")
	}
	user, err := selfUser(doc)
	if err != nil {
		return nil, err
	}
	if user.ConsumeLimit == "" {
		return nil, fmt.Errorf("Self consume protection is missing its installmentFlag")
	}
	if err := ValidateConsumeLimit(user.ConsumeLimit.String()); err != nil {
		return nil, fmt.Errorf("Self consume protection returned an invalid installmentFlag")
	}
	return &ConsumeLimit{Limit: user.ConsumeLimit, Unlimited: decimalAmount(user.ConsumeLimit.String()) == "999999"}, nil
}

var consumeLimitPattern = regexp.MustCompile(`^(0|[1-9]\d*)(\.\d{1,3})?$`)

func ValidateConsumeLimit(limit string) error {
	if !consumeLimitPattern.MatchString(limit) {
		return fmt.Errorf("consume limit must be a nonnegative amount with at most three decimal places; 999999 means unlimited")
	}
	return nil
}

func (s *Session) SetConsumeProtect(ctx context.Context, limit string) (*ConsumeChange, error) {
	if err := ValidateConsumeLimit(limit); err != nil {
		return nil, err
	}
	result := &ConsumeChange{RequestedLimit: json.Number(limit), Outcome: NotSubmitted}
	doc, err := s.page(ctx, "service/consumeProtect")
	if err != nil {
		return result, err
	}
	form := findForm(doc, "/Self/service/changeConsumeProtect")
	if form == nil {
		return result, fmt.Errorf("Self consume protection form is missing")
	}
	values := formValues(form)
	if values.Get("csrftoken") == "" {
		return result, fmt.Errorf("Self consume protection form is missing its CSRF token")
	}
	values.Set("consumeLimit", limit)
	result.Outcome = Unknown
	data, final, _, err := s.navigate(ctx, http.MethodPost, attr(form, "action"), values, "/Self/service/consumeProtect")
	if err != nil {
		return result, fmt.Errorf("consume limit submission failed; result is unknown: %w", err)
	}
	doc, err = s.authenticatedPage(data, final)
	if err != nil {
		return result, err
	}
	var current *ConsumeLimit
	if pathWithoutSession(final.Path) == "/Self/service/consumeProtect" {
		current, err = consumeProtect(doc)
	} else {
		current, err = s.ConsumeProtect(ctx)
	}
	if err != nil {
		return result, fmt.Errorf("consume limit was submitted; final state is unverified: %w", err)
	}
	result.ConsumeLimit = current
	if decimalAmount(current.Limit.String()) != decimalAmount(limit) {
		return result, fmt.Errorf("Self did not apply the requested consume limit")
	}
	result.Outcome, result.Verified = Accepted, true
	return result, nil
}

func decimalAmount(value string) string {
	if strings.Contains(value, ".") {
		value = strings.TrimRight(value, "0")
		value = strings.TrimSuffix(value, ".")
	}
	return value
}
