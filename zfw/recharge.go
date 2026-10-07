package zfw

import (
	"context"
	"fmt"

	"golang.org/x/net/html"
)

type RechargeAvailability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

// Recharge inspects the existing entry. This deployment has no payment form.
func (s *Session) Recharge(ctx context.Context) (*RechargeAvailability, error) {
	doc, err := s.page(ctx, "service/userRecharge")
	if err != nil {
		return nil, err
	}
	if find(doc, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "form" }) != nil {
		return nil, fmt.Errorf("Self recharge now contains a form; its payment contract requires examination")
	}
	return &RechargeAvailability{Available: false, Reason: "Self serves the service menu without a recharge form"}, nil
}
