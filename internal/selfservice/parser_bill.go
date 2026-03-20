package selfservice

import (
	"strconv"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func parseBillListResult(body []byte) (*kernel.BillListResult, error) {
	var payload struct {
		Summary map[string]interface{}   `json:"summary"`
		Total   interface{}              `json:"total"`
		Rows    []map[string]interface{} `json:"rows"`
	}
	if err := parseJSON(body, &payload); err != nil {
		return nil, err
	}
	if payload.Summary == nil {
		payload.Summary = map[string]interface{}{}
	}
	if payload.Rows == nil {
		payload.Rows = []map[string]interface{}{}
	}
	total, _ := strconv.Atoi(kernel.ToString(payload.Total))
	return &kernel.BillListResult{
		Summary: payload.Summary,
		Total:   total,
		Rows:    payload.Rows,
	}, nil
}
