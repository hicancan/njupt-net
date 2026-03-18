package selfservice

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

// GetUserOnlineLog loads the detailed online log bill endpoint.
func (c *Client) GetUserOnlineLog(ctx context.Context, startTime, endTime string) (*kernel.OperationResult[kernel.BillListResult], error) {
	return c.getBillList(ctx, "bill.onlineLog", "/Self/bill/getUserOnlineLog", startTime, endTime)
}

// GetMonthPay loads the month pay bill endpoint.
func (c *Client) GetMonthPay(ctx context.Context, startTime, endTime string) (*kernel.OperationResult[kernel.BillListResult], error) {
	return c.getBillList(ctx, "bill.monthPay", "/Self/bill/getMonthPay", startTime, endTime)
}

// GetOperatorLog loads the operator log bill endpoint.
func (c *Client) GetOperatorLog(ctx context.Context, startTime, endTime string) (*kernel.OperationResult[kernel.BillListResult], error) {
	return c.getBillList(ctx, "bill.operatorLog", "/Self/bill/getOperatorLog", startTime, endTime)
}

func (c *Client) getBillList(ctx context.Context, op, path, startTime, endTime string) (*kernel.OperationResult[kernel.BillListResult], error) {
	if err := c.ensureSession(op); err != nil {
		return nil, err
	}
	query := map[string]string{
		"pageSize":   "100",
		"pageNumber": "1",
		"sortName":   "loginTime",
		"sortOrder":  "desc",
		"_":          strconv.FormatInt(time.Now().UnixMilli(), 10),
	}
	if startTime != "" {
		query["startTime"] = startTime
	}
	if endTime != "" {
		query["endTime"] = endTime
	}

	resp, err := c.session.Get(ctx, path, kernel.RequestOptions{Query: query})
	if err != nil {
		return nil, &kernel.OpError{Op: op, Message: "request failed", Err: err}
	}

	var payload struct {
		Summary map[string]interface{}   `json:"summary"`
		Total   interface{}              `json:"total"`
		Rows    []map[string]interface{} `json:"rows"`
	}
	if err := parseJSON(resp.Body, &payload); err != nil {
		return nil, &kernel.OpError{Op: op, Message: "parse json failed", Err: err}
	}

	total, _ := strconv.Atoi(toString(payload.Total))
	data := &kernel.BillListResult{
		Summary: payload.Summary,
		Total:   total,
		Rows:    payload.Rows,
	}
	return &kernel.OperationResult[kernel.BillListResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: fmt.Sprintf("loaded %d bill rows", len(payload.Rows)),
		Data:    data,
		Raw:     rawCapture(resp),
	}, nil
}
