package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"

	"github.com/temporalio/orders-reference-app-go/app/fraudcheck"
	"go.temporal.io/sdk/activity"
)

// Activities implements the billing package's Activities.
// Any state shared by the worker among the activities is stored here.
type Activities struct {
	FraudCheckURL string
}

var a Activities

const (
	metricFraudcheck         = "fraudcheck"
	metricFraudcheckDeclined = "fraudcheck_declined"
	metricCharge             = "charge"
	metricChargeDeclined     = "charge_declined"
)

// GenerateInvoice activity creates an invoice for a fulfillment.
func (a *Activities) GenerateInvoice(_ context.Context, input *GenerateInvoiceInput) (*GenerateInvoiceResult, error) {
	var result GenerateInvoiceResult

	if input.CustomerID == "" {
		return nil, fmt.Errorf("CustomerID is required")
	}
	if input.Reference == "" {
		return nil, fmt.Errorf("OrderReference is required")
	}
	if len(input.Items) == 0 {
		return nil, fmt.Errorf("invoice must have items")
	}

	result.InvoiceReference = input.Reference

	for _, item := range input.Items {
		cost, tax := calculateCosts(item)
		result.SubTotal += cost
		result.Tax += tax
		result.Shipping += calculateShippingCost(item)
		result.Total += result.SubTotal + result.Tax + result.Shipping
	}

	return &result, nil
}

// calculateCosts calculates the cost and tax for an item.
func calculateCosts(item Item) (cost int32, tax int32) {
	// This is just a simulation, so make up a cost
	// Normally this would be looked up on the SKU
	costPerUnit := 3500 + rand.Int31n(8500)
	// Return tax at 20%
	return costPerUnit * int32(item.Quantity), costPerUnit * int32(item.Quantity) / 5
}

// calculateShippingCost calculates the shipping cost for an item.
func calculateShippingCost(item Item) int32 {
	// This is just a simulation, so make up a cost
	// Normally this would be looked up on the SKU
	costPerUnit := 500 + rand.Int31n(500)
	return costPerUnit * int32(item.Quantity)
}

func (a *Activities) fraudCheck(ctx context.Context, input *ChargeCustomerInput) (*fraudcheck.FraudCheckResult, error) {
	if a.FraudCheckURL == "" {
		return &fraudcheck.FraudCheckResult{Declined: false}, nil
	}

	checkInput := fraudcheck.FraudCheckInput{
		CustomerID: input.CustomerID,
		Charge:     input.Charge,
	}
	jsonInput, err := json.Marshal(checkInput)
	if err != nil {
		return nil, fmt.Errorf("failed to encode input: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.FraudCheckURL+"/check", bytes.NewReader(jsonInput))
	if err != nil {
		return nil, fmt.Errorf("failed to build fraud check request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := http.DefaultClient
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("fraud check request failed: %s: %s", http.StatusText(res.StatusCode), body)
	}

	var checkResult fraudcheck.FraudCheckResult

	err = json.NewDecoder(res.Body).Decode(&checkResult)
	if err == nil {
		handler := activity.GetMetricsHandler(ctx)
		handler.Counter(metricFraudcheck).Inc(1)
		if checkResult.Declined {
			handler.Counter(metricFraudcheckDeclined).Inc(1)
		}
	}

	return &checkResult, err
}

// ChargeCustomer activity charges a customer for a fulfillment.
func (a *Activities) ChargeCustomer(ctx context.Context, input *ChargeCustomerInput) (*ChargeCustomerResult, error) {
	var result ChargeCustomerResult

	checkResult, err := a.fraudCheck(ctx, input)
	if err != nil {
		return nil, err
	}

	result.Success = !checkResult.Declined
	result.AuthCode = "1234"

	handler := activity.GetMetricsHandler(ctx)
	handler.Counter(metricCharge).Inc(1)
	if !result.Success {
		handler.Counter(metricChargeDeclined).Inc(1)
	}

	activity.GetLogger(ctx).Info(
		"Charge",
		"Customer", input.CustomerID,
		"Amount", input.Charge,
		"Reference", input.Reference,
		"Success", result.Success,
	)

	return &result, nil
}
